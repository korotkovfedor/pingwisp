package poller

import (
	"context"
	"testing"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type checkFunc func(context.Context, models.Target) models.CheckResult

func (f checkFunc) Check(ctx context.Context, target models.Target) models.CheckResult {
	return f(ctx, target)
}

func TestCreateTargetStartsImmediatelyAndPreservesCreationSnapshot(t *testing.T) {
	started := make(chan struct{})
	p := New(checkFunc(func(ctx context.Context, target models.Target) models.CheckResult {
		close(started)
		statusCode := 200
		return models.CheckResult{
			Status:      models.StatusUp,
			StatusCode:  &statusCode,
			CompletedAt: time.Now(),
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("poller did not stop")
		}
	})

	created := p.CreateTarget("https://example.com/health", 24*time.Hour)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first check did not start immediately")
	}

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		current, ok := p.GetTarget(created.Settings.ID)
		if !ok {
			t.Fatal("created target disappeared")
		}
		if current.LastCheck != nil {
			if current.LastCheck.Status != "up" {
				t.Fatalf("first check status = %q; want up", current.LastCheck.Status)
			}
			if current.NextCheckAt == nil || !current.NextCheckAt.Equal(current.LastCheck.CompletedAt.Add(24*time.Hour)) {
				t.Fatalf("next check does not match completion time plus interval: %+v", current)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("first check result was not saved")
		}
	}

	if created.Settings.ID != 1 || created.LastCheck != nil || created.NextCheckAt == nil {
		t.Fatalf("creation snapshot changed after the first check: %+v", created)
	}
}
