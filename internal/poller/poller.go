package poller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type Poller struct {
	mu     sync.Mutex
	wakeup chan struct{}
	states map[models.TargetID]models.TargetState
	// TODO: Replace with min-heap
	scheduleq []pollTask
	active    map[models.TargetID]context.CancelFunc
	client    *http.Client

	nextID models.TargetID
}

type pollTask struct {
	executeAt time.Time
	targetID  models.TargetID
}

func New() *Poller {
	client := &http.Client{}

	return &Poller{
		wakeup: make(chan struct{}, 1),
		states: make(map[models.TargetID]models.TargetState),
		active: make(map[models.TargetID]context.CancelFunc),
		client: client,
	}
}

func (p *Poller) Run(ctx context.Context) {
	for {
		p.mu.Lock()
		var timer <-chan time.Time
		if len(p.scheduleq) != 0 {
			timerExecuteAt := p.scheduleq[0].executeAt
			timer = time.After(time.Until(timerExecuteAt))
		}
		p.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-p.wakeup:
			continue
		case <-timer:
			p.mu.Lock()
			if len(p.scheduleq) == 0 {
				p.mu.Unlock()
				continue
			}

			task := p.scheduleq[0]

			if time.Now().Before(task.executeAt) {
				p.mu.Unlock()
				continue
			}

			clear(p.scheduleq[:1])
			p.scheduleq = p.scheduleq[1:]
			state, ok := p.states[task.targetID]
			if !ok {
				p.mu.Unlock()
				slog.Warn("missing state after schedule")
				continue
			}

			ctx, cancel := context.WithCancel(ctx)
			p.active[task.targetID] = cancel
			p.states[state.Settings.ID] = models.TargetState{
				Settings:    state.Settings,
				LastCheck:   state.LastCheck,
				NextCheckAt: nil,
			}

			p.mu.Unlock()

			go func() {
				defer cancel()

				result := p.executePoll(ctx, state.Settings)

				p.mu.Lock()
				defer p.mu.Unlock()
				defer delete(p.active, state.Settings.ID)

				if ctx.Err() != nil {
					return
				}

				_, ok := p.states[state.Settings.ID]
				if !ok {
					return
				}

				nextCheckAt := result.CompletedAt.Add(state.Settings.Interval)

				p.states[state.Settings.ID] = models.TargetState{
					Settings:    state.Settings,
					LastCheck:   &result,
					NextCheckAt: &nextCheckAt,
				}
				p.scheduleq = append(p.scheduleq, pollTask{
					targetID:  state.Settings.ID,
					executeAt: nextCheckAt,
				})

				p.sortScheduled()
				p.notify()
			}()
		}
	}
}

func (p *Poller) CreateTarget(url string, interval time.Duration) models.Target {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := p.nextID
	p.nextID++
	target := models.Target{
		ID:       id,
		URL:      url,
		Interval: interval,
	}
	nextCheckAt := time.Now().Add(target.Interval)

	p.states[id] = models.TargetState{
		Settings:    target,
		NextCheckAt: &nextCheckAt,
	}
	p.scheduleq = append(p.scheduleq, pollTask{
		targetID:  target.ID,
		executeAt: nextCheckAt,
	})

	p.sortScheduled()
	p.notify()

	return target
}

func (p *Poller) DeleteTarget(id models.TargetID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.states[id]
	if !ok {
		return errors.New("target does not exist")
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

	return nil
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

func (p *Poller) executePoll(ctx context.Context, target models.Target) models.CheckResult {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()
	logger := slog.With(
		"url", target.URL,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", target.URL, nil)
	if err != nil {
		logger.Error(
			"poll failed",
			"error", err,
		)
		return models.CheckResult{
			Status:      models.StatusDown,
			CompletedAt: time.Now(),
			Error:       err,
		}
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		logger.Error(
			"poll failed",
			"error", err,
		)
		return models.CheckResult{
			Status:      models.StatusDown,
			CompletedAt: time.Now(),
			Error:       err,
			Latency:     time.Since(start),
		}
	}
	defer resp.Body.Close()

	const maxReadBytes = 1 * 1024 * 1024 // 1 MB
	limited := io.LimitReader(resp.Body, maxReadBytes+1)

	n, err := io.Copy(io.Discard, limited)
	latency := time.Since(start)
	if err != nil {
		logger.Error(
			"response read failed",
			"error", err,
		)
		return models.CheckResult{
			Status:      models.StatusDown,
			StatusCode:  &resp.StatusCode,
			CompletedAt: time.Now(),
			Latency:     latency,
			Error:       err,
		}
	}

	if n > maxReadBytes {
		logger.Error(
			"response limit exceeded",
		)
		return models.CheckResult{
			Status:      models.StatusDown,
			StatusCode:  &resp.StatusCode,
			CompletedAt: time.Now(),
			Latency:     latency,
			Error:       errors.New("response limit exceeded"),
		}
	}

	logger.Info(
		"poll completed",
		"status_code", resp.StatusCode,
	)

	var status string
	if 200 <= resp.StatusCode && resp.StatusCode < 300 {
		status = models.StatusUp
	} else {
		status = models.StatusDown
	}

	return models.CheckResult{
		Status:      status,
		StatusCode:  &resp.StatusCode,
		CompletedAt: time.Now(),
		Latency:     latency,
	}
}
