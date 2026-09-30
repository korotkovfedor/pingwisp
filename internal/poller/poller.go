package poller

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

// Checker performs one target check. Check may be called concurrently and must
// honor context cancellation.
type Checker interface {
	Check(ctx context.Context, target models.Target) models.CheckResult
}

type Poller struct {
	mu        sync.Mutex
	wakeup    chan struct{}
	states    map[models.TargetID]models.TargetState
	scheduleq []pollTask
	active    map[models.TargetID]context.CancelFunc
	checker   Checker

	nextID models.TargetID
}

type pollTask struct {
	executeAt time.Time
	targetID  models.TargetID
}

func New(checker Checker) *Poller {
	return &Poller{
		wakeup:  make(chan struct{}, 1),
		states:  make(map[models.TargetID]models.TargetState),
		active:  make(map[models.TargetID]context.CancelFunc),
		checker: checker,
		nextID:  1,
	}
}

func (p *Poller) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wakeup:
			continue
		case <-p.nextCheckTimer():
			p.startDueCheck(ctx)
		}
	}
}

func (p *Poller) nextCheckTimer() <-chan time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.scheduleq) == 0 {
		return nil
	}

	return time.After(time.Until(p.scheduleq[0].executeAt))
}

func (p *Poller) startDueCheck(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ctx.Err() != nil || len(p.scheduleq) == 0 {
		return
	}

	task := p.scheduleq[0]
	if time.Now().Before(task.executeAt) {
		return
	}

	clear(p.scheduleq[:1])
	p.scheduleq = p.scheduleq[1:]
	state, ok := p.states[task.targetID]
	if !ok {
		slog.Warn("missing state after schedule", "target_id", task.targetID)
		return
	}

	checkCtx, cancel := context.WithCancel(ctx)
	p.active[task.targetID] = cancel
	state.NextCheckAt = nil
	p.states[task.targetID] = state

	go p.runCheck(checkCtx, state.Settings, cancel)
}

func (p *Poller) runCheck(ctx context.Context, target models.Target, cancel context.CancelFunc) {
	defer cancel()

	result := p.checker.Check(ctx, target)
	if !p.finishCheck(ctx, target, result) {
		return
	}

	logger := slog.With(
		"target_id", target.ID,
		"url", target.URL,
		"status", result.Status,
		"latency_ms", result.Latency.Milliseconds(),
	)
	if result.StatusCode != nil {
		logger = logger.With("status_code", *result.StatusCode)
	}
	if result.Error != nil {
		logger.Error("poll failed", "error", result.Error)
	} else {
		logger.Info("poll completed")
	}
}

func (p *Poller) finishCheck(ctx context.Context, target models.Target, result models.CheckResult) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer delete(p.active, target.ID)

	if ctx.Err() != nil {
		return false
	}
	if _, ok := p.states[target.ID]; !ok {
		return false
	}

	nextCheckAt := result.CompletedAt.Add(target.Interval)
	p.states[target.ID] = models.TargetState{
		Settings:    target,
		LastCheck:   &result,
		NextCheckAt: &nextCheckAt,
	}
	p.scheduleq = append(p.scheduleq, pollTask{
		targetID:  target.ID,
		executeAt: nextCheckAt,
	})
	p.sortScheduled()
	p.notify()
	return true
}

func (p *Poller) CreateTarget(url string, interval time.Duration) models.TargetState {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := p.nextID
	p.nextID++
	target := models.Target{
		ID:       id,
		URL:      url,
		Interval: interval,
	}
	nextCheckAt := time.Now()

	state := models.TargetState{
		Settings:    target,
		NextCheckAt: &nextCheckAt,
	}
	p.states[id] = state
	p.scheduleq = append(p.scheduleq, pollTask{
		targetID:  target.ID,
		executeAt: nextCheckAt,
	})

	p.sortScheduled()
	p.notify()

	return state
}

func (p *Poller) DeleteTarget(id models.TargetID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.states[id]
	if !ok {
		return false
	}

	delete(p.states, id)
	p.scheduleq = slices.DeleteFunc(
		p.scheduleq,
		func(task pollTask) bool {
			return task.targetID == id
		},
	)

	cancel, ok := p.active[id]
	if ok {
		cancel()
		delete(p.active, id)
	}

	p.notify()

	return true
}

func (p *Poller) GetTarget(id models.TargetID) (models.TargetState, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	target, ok := p.states[id]
	return target, ok
}

func (p *Poller) GetTargets() []models.TargetState {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Collect(maps.Values(p.states))
}

func (p *Poller) sortScheduled() {
	slices.SortFunc(
		p.scheduleq,
		func(a, b pollTask) int {
			return a.executeAt.Compare(b.executeAt)
		},
	)
}

func (p *Poller) notify() {
	select {
	case p.wakeup <- struct{}{}:
	default:
	}
}
