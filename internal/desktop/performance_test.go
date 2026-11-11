package desktop_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

// The same generated sessions drive native memory and these CPU measurements:
// python3 scripts/perf-macos.py fixtures /tmp/yohaku-perf
// YOHAKU_PERF_FIXTURES=/tmp/yohaku-perf go test ./internal/desktop -run '^$' -bench BenchmarkPolish -benchmem -count=3
// Tester measures CPU-side software layout/drawing, not native input-to-screen.
func BenchmarkPolish(b *testing.B) {
	root := os.Getenv("YOHAKU_PERF_FIXTURES")
	if root == "" {
		b.Skip("generate fixtures and set YOHAKU_PERF_FIXTURES")
	}
	for _, scenario := range []string{"blank", "text10", "code30", "images"} {
		b.Run(scenario, func(b *testing.B) {
			source := notebook.Store{Dir: filepath.Join(root, scenario, "yohaku")}
			b.Run("Restore", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := source.Load(); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("CaptureSave", func(b *testing.B) {
				book, err := source.Load()
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					_ = book.CaptureSave()
				}
			})
			b.Run("Save", func(b *testing.B) {
				book, err := source.Load()
				if err != nil {
					b.Fatal(err)
				}
				store := notebook.Store{Dir: b.TempDir()}
				b.ReportAllocs()
				for b.Loop() {
					if err := store.Save(book); err != nil {
						b.Fatal(err)
					}
				}
			})
			for _, action := range []string{"Switch", "InputUndo"} {
				b.Run(action, func(b *testing.B) {
					book, err := source.Load()
					if err != nil {
						b.Fatal(err)
					}
					app := desktop.New(book, notebook.Store{Dir: b.TempDir()})
					defer app.Close()
					tt := ui.NewTester(app.View, 1080, 720)
					if err := app.Flush(); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					for b.Loop() {
						if action == "Switch" {
							book.Switch((book.Active + 1) % len(book.Open))
							tt.Frame()
						} else {
							book.Current().State.Insert("x")
							tt.Frame()
							book.Current().State.Run(editor.Undo)
							tt.Frame()
						}
					}
				})
			}
		})
	}
}

// BenchmarkFirstImageFrame isolates the first software-rendered image frame
// after restoring a session. Unlike repeated Switch, it cannot hide the cost
// of preparing a previously unpainted attachment. It is not native latency.
func BenchmarkFirstImageFrame(b *testing.B) {
	root := os.Getenv("YOHAKU_PERF_FIXTURES")
	if root == "" {
		b.Skip("generate fixtures and set YOHAKU_PERF_FIXTURES")
	}
	store := notebook.Store{Dir: filepath.Join(root, "images", "yohaku")}
	for b.Loop() {
		b.StopTimer()
		book, err := store.Load()
		if err != nil {
			b.Fatal(err)
		}
		book.Switch(1) // First rich note; the fixture opens on a text control.
		app := desktop.New(book, notebook.Store{Dir: b.TempDir()})
		b.StartTimer()
		tt := ui.NewTester(app.View, 1080, 720)
		_ = tt.Image()
		b.StopTimer()
		app.Close()
		b.StartTimer()
	}
}
