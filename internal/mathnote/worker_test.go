package mathnote

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"weak"
)

func workerResult(t *testing.T, w *Worker) Response {
	t.Helper()
	select {
	case r := <-w.Results:
		return r
	case <-time.After(30 * time.Second): // Include cold math.js VM startup under -race.
		t.Fatal("Math worker stalled")
		return Response{}
	}
}

func collected(t *testing.T, ref weak.Pointer[Engine], want bool) {
	t.Helper()
	runtime.GC()
	runtime.GC()
	if got := ref.Value() == nil; got != want {
		t.Fatalf("Engine collected = %v, want %v", got, want)
	}
}

func TestWorkerRetirementPreservesResults(t *testing.T) {
	const source = "x1 = 111+222\nx1*2 ="
	for _, tc := range []struct {
		name           string
		jobs, lines    int
		second         string
		secondReleased bool
	}{
		{"attempt budget", 2, 100, source, true},
		{"exact line budget", 100, 4, source, true},
		{"preflight larger note", 100, 4, source + "\nprev+1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := make(chan weak.Pointer[Engine], 3)
			w := newWorker(time.Hour, tc.jobs, tc.lines, func() (*Engine, error) {
				e, err := NewEngine()
				refs <- weak.Make(e)
				return e, err
			})
			defer w.Close()
			w.Submit(1, []Block{{Text: source}})
			first := workerResult(t, w)
			ref := <-refs
			if first.Err != nil || len(first.Results) != 2 || first.Results[1].Copy != "666" || !first.Results[1].Complete {
				t.Fatal(first)
			}
			collected(t, ref, false) // Releasing after every task is not reuse.
			w.Submit(2, []Block{{Text: tc.second}})
			second := workerResult(t, w)
			want := "666"
			if tc.second != source {
				want = "667"
			}
			if second.Err != nil || second.Results[len(second.Results)-1].Copy != want {
				t.Fatal(second)
			}
			collected(t, ref, true)
			if !tc.secondReleased {
				ref = <-refs // Preflight retires the old VM, retaining the new one.
				collected(t, ref, false)
			}
			w.Submit(3, []Block{{Text: source}})
			third := workerResult(t, w)
			if third.Err != nil || len(third.Results) != 2 || third.Results[1] != first.Results[1] {
				t.Fatal("reconstruction lost variables or completion", third)
			}
			w.Close()
			<-w.done
			collected(t, ref, true)
		})
	}
}

func TestWorkerIdleReleaseAndCancelledAttempts(t *testing.T) {
	refs := make(chan weak.Pointer[Engine], 4)
	create := func() (*Engine, error) {
		e, err := NewEngine()
		refs <- weak.Make(e)
		return e, err
	}
	w := newWorker(30*time.Millisecond, 100, 36000, create)
	defer w.Close()
	w.Submit(1, []Block{{Text: "13473.05+845.96"}})
	first := workerResult(t, w)
	ref := <-refs
	deadline := time.Now().Add(10 * time.Second)
	for ref.Value() != nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		runtime.GC()
	}
	collected(t, ref, true)
	w.Submit(2, []Block{{Text: "13473.05+845.96"}})
	second := workerResult(t, w)
	if first.Err != nil || second.Err != nil || len(second.Results) != 1 || first.Results[0] != second.Results[0] || second.Results[0].Copy != "14319.01" {
		t.Fatal("idle release lost result", first, second)
	}
	w.Close()
	<-w.done
	collected(t, <-refs, true)

	started, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	w = newWorker(time.Hour, 2, 36000, func() (*Engine, error) {
		once.Do(func() { close(started); <-gate })
		return create()
	})
	defer w.Close()
	w.Submit(1, []Block{{Text: "old = 9"}})
	<-started
	w.Submit(2, []Block{{Text: "old+1\n12x3"}})
	close(gate)
	r := workerResult(t, w)
	if r.Generation != 2 || r.Err != nil || len(r.Results) != 2 || r.Results[0].Error == "" || r.Results[1].Copy != "36" {
		t.Fatal("cancel leaked scope or stale result", r)
	}
	// The cancelled attempt counts: a budget of two retires after the latest
	// successful job, without needing idle or Close to free the first Engine.
	collected(t, <-refs, true)
	w.Close()
	<-w.done

	// Rejected input must consume an attempted-job budget too, not keep a
	// previously warmed VM indefinitely when every subsequent job is invalid.
	w = newWorker(time.Hour, 2, 36000, create)
	defer w.Close()
	w.Submit(1, []Block{{Text: "1+2"}})
	workerResult(t, w)
	ref = <-refs
	w.Submit(2, []Block{{Text: strings.Repeat("1\n", 2001)}})
	if r := workerResult(t, w); r.Err == nil {
		t.Fatal("note limit missing")
	}
	collected(t, ref, true)
}
