package internal

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
)

type TargetID uint64

type Poller struct {
	mu      sync.Mutex
	wakeup  chan struct{}
	targets map[TargetID]Target
	// TODO: Replace with min-heap
	scheduleq []pollTask
	client    *http.Client
}

type Target struct {
	ID       TargetID
	URL      string
	Interval time.Duration
}

type pollTask struct {
	executeAt time.Time
	targetID  TargetID
}

func NewPoller() *Poller {
	client := &http.Client{}

	return &Poller{
		wakeup:  make(chan struct{}, 1),
		targets: make(map[TargetID]Target),
		client:  client,
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
			target, ok := p.targets[task.targetID]
			if !ok {
				p.mu.Unlock()
				slog.Warn("missing target after schedule")
				continue
			}
			p.mu.Unlock()

			go func() {
				p.executePoll(ctx, target)
				if ctx.Err() != nil {
					return
				}

				p.mu.Lock()
				defer p.mu.Unlock()

				_, ok := p.targets[target.ID]
				if !ok {
					return
				}

				p.scheduleq = append(p.scheduleq, pollTask{
					targetID:  target.ID,
					executeAt: time.Now().Add(target.Interval),
				})

				p.sortScheduled()
				p.notify()

			}()
		}
	}
}

func (p *Poller) AddTarget(target Target) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.targets[target.ID]
	if ok {
		return errors.New("target already exists")
	}

	p.targets[target.ID] = target
	p.scheduleq = append(p.scheduleq, pollTask{
		targetID:  target.ID,
		executeAt: time.Now().Add(target.Interval),
	})

	p.sortScheduled()
	p.notify()

	return nil
}

func (p *Poller) DeleteTarget(id TargetID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.targets[id]
	if !ok {
		return errors.New("target does not exist")
	}

	delete(p.targets, id)
	p.scheduleq = slices.DeleteFunc(
		p.scheduleq,
		func(task pollTask) bool {
			return task.targetID == id
		},
	)

	p.notify()

	return nil
}

func (p *Poller) GetTarget(id TargetID) (Target, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	target, ok := p.targets[id]
	if !ok {
		return Target{}, errors.New("target does not exist")
	}

	return target, nil

}

func (p *Poller) GetTargets() []Target {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Collect(maps.Values(p.targets))
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

func (p *Poller) executePoll(ctx context.Context, target Target) {
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
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Error(
			"poll failed",
			"error", err,
		)
		return
	}
	defer resp.Body.Close()

	const maxReadBytes = 1 * 1024 * 1024 // 1 MB
	limited := io.LimitReader(resp.Body, maxReadBytes+1)

	n, err := io.Copy(io.Discard, limited)
	if err != nil {
		logger.Error(
			"response read failed",
			"error", err,
		)
		return
	}

	if n > maxReadBytes {
		logger.Error(
			"response limit exceeded",
		)
		return
	}

	logger.Info(
		"poll completed",
		"status_code", resp.StatusCode,
	)
}
