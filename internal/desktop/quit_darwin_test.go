package desktop_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

const quitTestText = "⟦yohaku:Plain Text⟧\nQuit without rendering 中文😀"

func TestMain(m *testing.M) {
	if dir := os.Getenv("YOHAKU_TEST_QUIT_DIR"); dir != "" {
		// Run the real native event loop on the main goroutine, in an
		// isolated process. No view ever polls the save result here.
		mygo.App.WhenReady(func() {
			book := notebook.NewBook()
			n, err := notebook.NewNote("Quit regression", quitTestText)
			if err != nil {
				panic(err)
			}
			book.Open[0] = n
			a := desktop.New(book, notebook.Store{Dir: dir})
			a.Window = mygo.NewWindow(mygo.WindowOptions{
				Hidden: true, Content: ui.View(func(*ui.Context) {}),
			})
			mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
				if !a.PrepareClose(mygo.App.Quit) {
					e.PreventDefault()
				} else {
					a.Close()
				}
			})
			mygo.App.Quit()
		})
		if err := mygo.App.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNativeQuitSavesWithoutRendering(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "YOHAKU_TEST_QUIT_DIR="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native quit must finish without a rendered frame: %v\n%s", err, output)
	}
	loaded, err := (notebook.Store{Dir: dir}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Current().State.Buffer.String(); got != quitTestText {
		t.Fatalf("quit did not durably save the note: %q", got)
	}
}
