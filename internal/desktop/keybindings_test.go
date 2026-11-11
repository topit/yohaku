package desktop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func TestBindingsRecordConflictSaveRestoreAndScope(t *testing.T) {
	dir := t.TempDir()
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: dir})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("original")
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Shortcuts")
	click(t, tt, "Search shortcuts")
	tt.Type("block.add")
	click(t, tt, "Shortcut block.add")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyN)
	if !tt.HasText("Shortcut conflict: New block note / Add block") || len(b.Open) != 1 {
		t.Fatal("conflict executed a command or was not shown", tt.Texts())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	tt.Frame()
	click(t, tt, "Save settings")
	tt.Frame()
	n := b.Current()
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(n.Blocks()) != 1 {
		t.Fatal("old shortcut still creates blocks")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	if len(n.Blocks()) != 2 {
		t.Fatal("remapped shortcut not applied")
	}
	// The block command must not run in a search input.
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	if len(n.Blocks()) != 2 {
		t.Fatal("block shortcut escaped editor scope")
	}
	tt.Key(0, ui.KeyEscape)
	data, _ := os.ReadFile(filepath.Join(dir, "preferences.json"))
	if !strings.Contains(string(data), `"block.add": "Cmd+Shift+B"`) {
		t.Fatal(string(data))
	}
	a2 := desktop.New(notebook.NewBook(), notebook.Store{Dir: dir})
	defer a2.Close()
	tt2 := ui.NewTester(a2.View, 1080, 720)
	tt2.Key(ui.Cmd|ui.Shift, ui.KeyB)
	if len(a2.Book.Current().Blocks()) != 2 {
		t.Fatal("binding lost on restart")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Shortcuts")
	click(t, tt, "Search shortcuts")
	tt.Type("block.add")
	click(t, tt, "Shortcut block.add")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyC)
	if !tt.HasText("Reserved editing or system shortcut: ⌘C") {
		t.Fatal("reserved editing accepted", tt.Texts())
	}
	click(t, tt, "Clear block.add")
	click(t, tt, "Cancel")
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	if len(n.Blocks()) != 3 {
		t.Fatal("cancel changed live binding")
	}
}

func TestBindingSaveFailureKeepsDraft(t *testing.T) {
	dir := t.TempDir()
	a := desktop.New(notebook.NewBook(), notebook.Store{Dir: dir})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Shortcuts")
	click(t, tt, "Search shortcuts")
	tt.Type("note.new")
	click(t, tt, "Shortcut note.new")
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyN)
	if err := os.Mkdir(filepath.Join(dir, "preferences.json"), 0700); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	tt.Frame()
	if !tt.HasText("⌘⇧N") || len(a.Book.Open) != 1 {
		t.Fatal("draft lost on save failure", tt.Texts())
	}
	if err := os.Remove(filepath.Join(dir, "preferences.json")); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyN)
	if len(a.Book.Open) != 1 {
		t.Fatal("old binding survives")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyN)
	if len(a.Book.Open) != 2 {
		t.Fatal("retry did not apply")
	}
}
