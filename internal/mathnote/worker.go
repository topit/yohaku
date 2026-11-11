package mathnote

import (
	"context"
	"strings"
	"time"
)

type Response struct {
	Generation uint64
	Results    []Result
	Err        error
}

type request struct {
	ctx        context.Context
	generation uint64
	blocks     []Block
}

// Worker owns a single VM. Submit/Cancel/Close run on the UI thread; only the
// immutable source snapshot crosses threads. Queues hold only the latest job.
type Worker struct {
	Results chan Response
	jobs    chan request
	ctx     context.Context
	stop    context.CancelFunc
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewWorker() *Worker {
	return newWorker(30*time.Second, 500, 36000, NewEngine)
}

// Keep lifecycle controls internal. Tests shorten the limits and observe real
// Engine collection without adding a public settings/configuration surface.
func newWorker(idle time.Duration, maxJobs, maxLines int, create func() (*Engine, error)) *Worker {
	ctx, stop := context.WithCancel(context.Background())
	w := &Worker{Results: make(chan Response, 1), jobs: make(chan request, 1), ctx: ctx, stop: stop, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		var engine *Engine
		attempts, lines := 0, 0
		timer := time.NewTimer(idle)
		timer.Stop()
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				// Expiry remains valid even if a cancelled job is queued: skipping
				// it must not consume the only timer event and retain the VM.
				engine, attempts, lines = nil, 0, 0
			case job := <-w.jobs:
				if job.ctx.Err() != nil {
					continue
				}
				timer.Stop()
				cost := 0
				for _, b := range job.blocks {
					cost += strings.Count(b.Text, "\n") + 1
				}
				// Preflight avoids overshooting the cumulative line budget with
				// a larger note. A task is never split across VMs.
				if attempts >= maxJobs || lines+cost > maxLines {
					engine, attempts, lines = nil, 0, 0
				}
				r := Response{Generation: job.generation}
				if engine == nil {
					engine, r.Err = create()
				}
				if r.Err == nil {
					attempts++
					lines += cost // Count attempted work, including failure/cancel.
					r.Results, r.Err = engine.Evaluate(job.ctx, job.blocks)
				}
				if attempts >= maxJobs || lines >= maxLines {
					engine, attempts, lines = nil, 0, 0
				}
				if engine != nil {
					timer.Reset(idle)
				}
				if job.ctx.Err() != nil {
					continue
				}
				select {
				case <-w.Results:
				default:
				}
				select {
				case w.Results <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return w
}

func (w *Worker) Cancel() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *Worker) Submit(generation uint64, blocks []Block) {
	w.Cancel()
	ctx, cancel := context.WithCancel(w.ctx)
	w.cancel = cancel
	select {
	case <-w.jobs:
	default:
	}
	w.jobs <- request{ctx, generation, blocks}
}

func (w *Worker) Close() { w.Cancel(); w.stop() }
