package poller

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type controlledCheck struct {
	ctx    context.Context
	target models.Target
	result chan models.CheckResult
}

type controlledChecker struct {
	calls chan controlledCheck
}

func newControlledChecker() *controlledChecker {
	return &controlledChecker{calls: make(chan controlledCheck, 8)}
}

func (c *controlledChecker) Check(ctx context.Context, target models.Target) models.CheckResult {
	call := controlledCheck{ctx: ctx, target: target, result: make(chan models.CheckResult, 1)}
	select {
	case c.calls <- call:
	case <-ctx.Done():
		return models.CheckResult{Status: models.StatusDown, CompletedAt: time.Now(), Error: ctx.Err()}
	}
	select {
	case result := <-call.result:
		return result
	case <-ctx.Done():
		return models.CheckResult{Status: models.StatusDown, CompletedAt: time.Now(), Error: ctx.Err()}
	}
}

func runTestPoller(t *testing.T, p *Poller) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go p.Run(ctx)
	return cancel
}

func nextCall(t *testing.T, c *controlledChecker) controlledCheck {
	t.Helper()
	select {
	case call := <-c.calls:
		return call
	case <-time.After(time.Minute):
		t.Fatal("check did not start")
		return controlledCheck{}
	}
}

func assertNoCall(t *testing.T, c *controlledChecker) {
	t.Helper()
	select {
	case call := <-c.calls:
		t.Fatalf("unexpected check for target %d", call.target.ID)
	default:
	}
}

func assertEmptyPoller(t *testing.T, p *Poller) {
	t.Helper()
	if states := p.GetTargets(); len(states) != 0 {
		t.Errorf("remaining targets: %+v", states)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.scheduleq) != 0 || len(p.active) != 0 {
		t.Errorf("remaining scheduled checks = %d, active checks = %d", len(p.scheduleq), len(p.active))
	}
}

func successfulResult() models.CheckResult {
	code := 200
	return models.CheckResult{Status: models.StatusUp, StatusCode: &code, CompletedAt: time.Now()}
}

func TestDeleteScheduledTarget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newControlledChecker()
		p := New(c)
		deleted := p.CreateTarget("http://example.com/deleted", time.Second)
		if !p.DeleteTarget(deleted.Settings.ID) || p.DeleteTarget(deleted.Settings.ID) {
			t.Fatal("deletion must succeed once")
		}
		if _, ok := p.GetTarget(deleted.Settings.ID); ok {
			t.Fatal("deleted target remains readable")
		}
		assertEmptyPoller(t, p)

		survivor := p.CreateTarget("http://example.com/survivor", time.Hour)
		runTestPoller(t, p)
		call := nextCall(t, c)
		if call.target.ID != survivor.Settings.ID {
			t.Fatalf("started target %d; want survivor %d", call.target.ID, survivor.Settings.ID)
		}
		call.result <- successfulResult()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assertNoCall(t, c)
	})
}

func TestDeleteActiveTarget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newControlledChecker()
		p := New(c)
		runTestPoller(t, p)
		created := p.CreateTarget("http://example.com/health", time.Second)
		call := nextCall(t, c)
		if !p.DeleteTarget(created.Settings.ID) {
			t.Fatal("active target was not deleted")
		}
		select {
		case <-call.ctx.Done():
		default:
			t.Fatal("active check context was not canceled")
		}
		synctest.Wait()
		assertEmptyPoller(t, p)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assertNoCall(t, c)
		if _, ok := p.GetTarget(created.Settings.ID); ok {
			t.Fatal("canceled check restored the deleted target")
		}
	})
}

func TestDeleteTargetBetweenChecks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newControlledChecker()
		p := New(c)
		deleted := p.CreateTarget("http://example.com/deleted", time.Second)
		survivor := p.CreateTarget("http://example.com/survivor", time.Hour)
		runTestPoller(t, p)
		for range 2 {
			call := nextCall(t, c)
			call.result <- successfulResult()
		}
		synctest.Wait()
		state, ok := p.GetTarget(deleted.Settings.ID)
		if !ok || state.LastCheck == nil || state.NextCheckAt == nil {
			t.Fatalf("target was not rescheduled after its first check: %+v", state)
		}
		if !p.DeleteTarget(deleted.Settings.ID) {
			t.Fatal("scheduled target was not deleted")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		assertNoCall(t, c)
		time.Sleep(time.Hour - time.Second)
		call := nextCall(t, c)
		if call.target.ID != survivor.Settings.ID {
			t.Fatalf("started target %d; want survivor %d", call.target.ID, survivor.Settings.ID)
		}
		call.result <- successfulResult()
		synctest.Wait()
		states := p.GetTargets()
		if len(states) != 1 || states[0].Settings.ID != survivor.Settings.ID {
			t.Fatalf("remaining targets = %+v; want only survivor", states)
		}
	})
}

func TestLateResultDoesNotRestoreDeletedTarget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan context.Context, 8)
		release := make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		t.Cleanup(unblock)
		p := New(checkFunc(func(ctx context.Context, target models.Target) models.CheckResult {
			started <- ctx
			// Simulate an in-flight result arriving after cancellation.
			<-release
			return successfulResult()
		}))
		runTestPoller(t, p)
		created := p.CreateTarget("http://example.com/health", time.Second)
		checkCtx := <-started
		if !p.DeleteTarget(created.Settings.ID) {
			t.Fatal("target was not deleted")
		}
		if checkCtx.Err() != context.Canceled {
			t.Fatalf("check context error = %v; want Canceled", checkCtx.Err())
		}
		unblock()
		synctest.Wait()
		assertEmptyPoller(t, p)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		select {
		case <-started:
			t.Fatal("late result scheduled another check")
		default:
		}
	})
}

func TestChecksDoNotOverlapAndKeepLastResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newControlledChecker()
		p := New(c)
		cancel := runTestPoller(t, p)
		created := p.CreateTarget("http://example.com/health", time.Second)
		first := nextCall(t, c)
		initial, _ := p.GetTarget(created.Settings.ID)
		if initial.LastCheck != nil || initial.NextCheckAt != nil {
			t.Fatalf("first active check state = %+v; want no result or scheduled time", initial)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		assertNoCall(t, c)

		result := successfulResult()
		first.result <- result
		synctest.Wait()
		completed, _ := p.GetTarget(created.Settings.ID)
		if completed.NextCheckAt == nil || !completed.NextCheckAt.Equal(result.CompletedAt.Add(time.Second)) {
			t.Fatalf("next check = %v; want completion + interval", completed.NextCheckAt)
		}
		time.Sleep(time.Second - time.Nanosecond)
		synctest.Wait()
		assertNoCall(t, c)
		time.Sleep(time.Nanosecond)
		second := nextCall(t, c)
		if second.target.ID != first.target.ID {
			t.Fatal("repeat check belongs to a different target")
		}
		during, _ := p.GetTarget(created.Settings.ID)
		if during.NextCheckAt != nil || !reflect.DeepEqual(during.LastCheck, &result) {
			t.Fatalf("second active check state = %+v; want previous result and no scheduled time", during)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		assertNoCall(t, c)
		cancel()
		synctest.Wait()
		afterCancel, _ := p.GetTarget(created.Settings.ID)
		if !reflect.DeepEqual(afterCancel.LastCheck, &result) {
			t.Fatalf("cancellation replaced the last result: %+v", afterCancel.LastCheck)
		}
	})
}

func TestConcurrentCreateReadDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := New(checkFunc(func(context.Context, models.Target) models.CheckResult { return successfulResult() }))
		runTestPoller(t, p)
		const workers, perWorker = 8, 50
		ids := make(chan models.TargetID, workers*perWorker)
		readState := func(state models.TargetState) {
			if state.Settings.ID == 0 || state.Settings.URL == "" {
				t.Errorf("incomplete target: %+v", state)
			}
			if next := state.NextCheckAt; next != nil && next.IsZero() {
				t.Error("next check time is zero")
			}
			if result := state.LastCheck; result != nil {
				if result.Status != models.StatusUp || result.StatusCode == nil || *result.StatusCode != 200 || result.CompletedAt.IsZero() || result.Error != nil {
					t.Errorf("incomplete check result: %+v", result)
				}
			}
		}
		var wg sync.WaitGroup
		for worker := range workers {
			wg.Go(func() {
				for i := range perWorker {
					url := fmt.Sprintf("http://example.com/%d/%d", worker, i)
					created := p.CreateTarget(url, time.Hour)
					id := created.Settings.ID
					ids <- id
					state, ok := p.GetTarget(id)
					if !ok || state.Settings.URL != url || state.Settings.ID != id {
						t.Errorf("inconsistent state for target %d: %+v, exists=%v", id, state, ok)
					}
					readState(state)
					for _, listed := range p.GetTargets() {
						readState(listed)
					}
					if !p.DeleteTarget(id) || p.DeleteTarget(id) {
						t.Errorf("target %d was not deleted exactly once", id)
					}
					if _, ok := p.GetTarget(id); ok {
						t.Errorf("deleted target %d is still readable", id)
					}
				}
			})
		}
		wg.Wait()
		close(ids)
		seen := make(map[models.TargetID]bool)
		for id := range ids {
			if id == 0 || seen[id] {
				t.Errorf("invalid or duplicate ID: %d", id)
			}
			seen[id] = true
		}
		if len(seen) != workers*perWorker {
			t.Errorf("unique IDs = %d; want %d", len(seen), workers*perWorker)
		}
		synctest.Wait()
		assertEmptyPoller(t, p)
	})
}
