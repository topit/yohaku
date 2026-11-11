package desktop

import (
	"time"

	"yohaku/internal/notebook"
)

type saveJob struct {
	snapshot notebook.SaveSnapshot
	revision uint64
}

type saveResult struct {
	job saveJob
	err error
}

type saveState struct {
	busy      bool
	revision  uint64
	results   chan saveResult
	pending   *saveJob
	after     func()
	exitReady bool
}

// Saving reports work in flight, not edits waiting for the autosave interval.
func (a *App) Saving() bool { return a.save.busy }

func (a *App) trackChanges() bool {
	if a.observe() {
		a.dirty = true
		a.save.revision++
		a.save.exitReady = false
		return true
	}
	return false
}

// RequestSave captures cheap immutable versions on the UI thread. At most
// one job writes and one latest snapshot waits; intermediate requests coalesce.
func (a *App) RequestSave() {
	a.trackChanges()
	a.save.revision++ // Explicit saves also capture reading positions.
	a.dirty = true
	job := saveJob{a.Book.CaptureSave(), a.save.revision}
	if a.save.busy {
		a.save.pending = &job
	} else {
		a.startSave(job)
	}
}

func (a *App) startSave(job saveJob) {
	if a.save.results == nil {
		a.save.results = make(chan saveResult, 1)
	}
	a.save.busy = true
	store, results, window := a.Store, a.save.results, a.Window
	go func() {
		err := store.WriteSnapshot(job.snapshot)
		results <- saveResult{job, err}
		if window != nil {
			// A hidden, minimized or occluded window may not draw another
			// frame. Acknowledge the save on the UI loop, not in View alone,
			// so a successful save can resume quitting without a redraw.
			window.Update(func() { a.pollSave(time.Now()) })
		}
	}()
}

func (a *App) completeSave(r saveResult) {
	a.save.busy = false
	a.trackChanges() // New input must be observed before acknowledging old work.
	if r.err != nil {
		a.SaveError, a.dirty = r.err.Error(), true
		a.save.pending, a.save.after = nil, nil
		a.due = time.Time{} // autosave applies the retry interval below.
		return
	}
	r.job.snapshot.Accept(a.Book, a.Store)
	a.SaveError = ""
	a.dirty = r.job.revision != a.save.revision
	if next := a.save.pending; next != nil {
		a.save.pending = nil
		a.startSave(*next)
		return
	}
	if a.save.after != nil {
		if a.dirty {
			a.RequestSave() // A close/open-folder gate must include intervening edits.
		} else {
			after := a.save.after
			a.save.after = nil
			after()
		}
	}
}

func (a *App) pollSave(now time.Time) {
	if a.save.busy {
		select {
		case r := <-a.save.results:
			a.completeSave(r)
			if r.err != nil {
				a.due = now.Add(2 * time.Second)
			}
		default:
		}
	}
}

// SaveThen runs an action on the UI thread only after the latest content is
// durable. An error cancels the action and leaves the editor available.
func (a *App) SaveThen(action func()) {
	a.save.after = action
	a.RequestSave()
}

// PrepareClose is an asynchronous native close/quit gate. The first event
// is prevented; resume retries it after the latest snapshot has been saved.
func (a *App) PrepareClose(resume func()) bool {
	a.trackChanges()
	if a.save.exitReady && !a.dirty && !a.save.busy {
		return true
	}
	a.SaveThen(func() { a.save.exitReady = true; resume() })
	return false
}

// Flush is the synchronous headless/tooling API. Native UI actions use
// RequestSave/SaveThen/PrepareClose and never wait on disk from the UI thread.
func (a *App) Flush() error {
	for a.save.busy {
		a.completeSave(<-a.save.results)
	}
	if err := a.Store.Save(a.Book); err != nil {
		a.SaveError, a.dirty = err.Error(), true
		return err
	}
	a.SaveError, a.dirty = "", false
	return nil
}
