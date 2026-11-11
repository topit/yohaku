package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"yohaku/internal/notebook"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv("YOHAKU_TEST_WINDOW_DIR"); dir != "" {
		os.Args = []string{os.Args[0], "-data-dir", dir}
		mygo.App.WhenReady(func() {
			go func() {
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				var window *mygo.Window
				phase := 0
				for range ticker.C {
					mygo.RunOnMain(func() {
						switch phase {
						case 0:
							if windows := mygo.Windows(); len(windows) == 1 && windows[0].IsVisible() {
								window = windows[0]
								phase = 1
								window.Close()
							}
						case 1:
							if !window.IsVisible() && !window.IsDestroyed() {
								if err := os.WriteFile(filepath.Join(dir, "window-kept"), []byte("hidden, not destroyed"), 0600); err != nil {
									panic(err)
								}
								phase = 2
								window.Show()
							}
						case 2:
							if window.IsVisible() {
								phase = 3
								mygo.App.Quit()
							}
						}
					})
				}
			}()
		})
		main() // Exercise the production close and quit handlers.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestMacWindowCloseKeepsWindowButQuitExits(t *testing.T) {
	dir := t.TempDir()
	store := notebook.Store{Dir: dir}
	book := notebook.NewBook()
	book.HeynoteImportHandled = true
	if err := store.Save(book); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Include cold native/font initialization under the race detector; this
	// checks lifecycle completion, not a ten-second startup performance SLA.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "YOHAKU_TEST_WINDOW_DIR="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("close/show/quit did not complete: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(dir, "window-kept")); err != nil {
		t.Fatal("closing the window exited the app or destroyed the editor", err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal("library could not be restored after quit", err)
	}
}
