package notebook_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/notebook"
)

func note(t *testing.T, text string) *notebook.Note {
	t.Helper()
	n, err := notebook.NewNote("Untitled", text)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestProtectedBoundaryRejectsWholeTransaction(t *testing.T) {
	text := notebook.Header("Go") + "alpha" + "\n" + notebook.Header("Python") + "中文😀"
	for _, mode := range []string{"selection", "multi-cursor", "buffer", "prefix", "assembled-prefix"} {
		t.Run(mode, func(t *testing.T) {
			n := note(t, text)
			blocks, _ := notebook.Blocks(text)
			n.State.Select(len(text), len(text))
			n.State.Insert("!")
			n.State.Run(editor.Undo)
			switch mode {
			case "selection":
				n.State.Select(blocks[0].End-1, blocks[1].Body+3)
			case "multi-cursor":
				n.State.SetSelections(editor.Selection{Anchor: blocks[0].Body, Head: blocks[0].Body + 1}, editor.Selection{Anchor: blocks[1].Start, Head: blocks[1].Body})
			case "prefix":
				n.State.Select(len(text), len(text))
			case "assembled-prefix":
				n.State.Select(len(text), len(text))
				n.State.Insert("⟦yo")
				n.State.Select(n.State.Buffer.Len(), n.State.Buffer.Len())
			}
			before, version, sels := n.State.Buffer.String(), n.State.Buffer.Version(), n.State.Selections()
			switch mode {
			case "buffer":
				if n.State.Buffer.TryEdit(editor.Edit{Start: blocks[0].End, End: blocks[1].Body}, editor.Edit{Start: blocks[0].Body, End: blocks[0].Body + 1, Text: "z"}) {
					t.Fatal("boundary batch accepted")
				}
			case "prefix":
				n.State.Insert(notebook.Header("JSON"))
			case "assembled-prefix":
				n.State.Insert("haku:")
			default:
				n.State.Insert("replacement")
			}
			if n.State.Buffer.String() != before || n.State.Buffer.Version() != version || !reflect.DeepEqual(n.State.Selections(), sels) || n.Message == "" {
				t.Fatal("rejection changed text/version/selection or omitted feedback")
			}
			if mode != "assembled-prefix" {
				n.State.Run(editor.Redo)
				if n.State.Buffer.String() != text+"!" {
					t.Fatal("rejection destroyed redo")
				}
			}
		})
	}
	// Both sides of the boundary: inserting before its separator and after
	// its header are legal, inserting inside the marker is not.
	n := note(t, text)
	blocks, _ := notebook.Blocks(text)
	if !n.State.Buffer.TryEdit(editor.Edit{Start: blocks[0].End, End: blocks[0].End, Text: " tail"}, editor.Edit{Start: blocks[1].Body, End: blocks[1].Body, Text: "head "}) {
		t.Fatal("body edge rejected")
	}
	if n.State.Buffer.String() != notebook.Header("Go")+"alpha tail\n"+notebook.Header("Python")+"head 中文😀" {
		t.Fatal("body edge text incorrect")
	}
}

func TestBlockCommandsAndUndo(t *testing.T) {
	n := note(t, notebook.Header("Go")+"first中文second")
	original := n.State.Buffer.String()
	n.State.Select(len(notebook.Header("Go")+"first中文"), len(notebook.Header("Go")+"first中文"))
	n.SplitBlock()
	split := notebook.Header("Go") + "first中文\n" + notebook.Header("Go") + "second"
	if n.State.Buffer.String() != split {
		t.Fatal("split lost body text")
	}
	n.SetLanguage("Python")
	if n.State.Buffer.String() != notebook.Header("Go")+"first中文\n"+notebook.Header("Python")+"second" {
		t.Fatal("wrong block language")
	}
	n.AddBlock()
	if n.State.Buffer.String() != notebook.Header("Go")+"first中文\n"+notebook.Header("Python")+"second\n"+notebook.Header("Plain Text") {
		t.Fatal("add position incorrect")
	}
	n.RemoveBlock()
	n.State.Run(editor.Undo) // restore removed block
	if blocks, err := notebook.Blocks(n.State.Buffer.String()); err != nil || len(blocks) != 3 {
		t.Fatal("undo remove broke structure")
	}
	n.State.Run(editor.Undo) // undo add
	n.State.Run(editor.Undo) // undo language
	if n.State.Buffer.String() != split {
		t.Fatal("language change not independently undoable")
	}
	n.State.Run(editor.Undo)
	if n.State.Buffer.String() != original {
		t.Fatal("split not undoable")
	}
	for _, index := range []int{0, 1, 2} {
		x := note(t, notebook.Header("Go")+"one\n"+notebook.Header("Python")+"two\n"+notebook.Header("JSON")+"three")
		blocks, _ := notebook.Blocks(x.State.Buffer.String())
		x.State.Select(blocks[index].Body, blocks[index].Body)
		x.RemoveBlock()
		remaining, err := notebook.Blocks(x.State.Buffer.String())
		if err != nil || len(remaining) != 2 {
			t.Fatalf("remove block %d: %v", index, err)
		}
		x.State.Insert("x") // caret must be in an editable body, not a header
		if x.Message != "" {
			t.Fatalf("remove %d left caret in header", index)
		}
	}
}

func TestTabsArchiveAndRestartRoundTrip(t *testing.T) {
	b := notebook.NewBook()
	first := b.Current()
	first.State.Insert("unsaved 中文😀")
	start := len(notebook.Header("Plain Text"))
	first.State.Select(start, start+3)
	b.New()
	b.Rename("Second")
	b.Current().State.Insert("second text")
	b.Switch(0)
	if first.State.Selection() != (editor.Selection{Anchor: start, Head: start + 3}) {
		t.Fatal("switch lost selection")
	}
	b.Close(0)
	b.Reopen()
	if b.Current() != first {
		t.Fatal("close/reopen replaced buffer")
	}
	first.State.Run(editor.Undo)
	if first.State.Buffer.String() != notebook.Header("Plain Text") {
		t.Fatal("close lost undo history")
	}
	first.State.Run(editor.Redo)
	b.Close(b.Active)
	store := notebook.Store{Dir: t.TempDir()}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), b.Snapshot()) {
		t.Fatal("restart changed open/archived notes or position")
	}
	restored.Reopen()
	if restored.Current().ID != first.ID || restored.Current().Title != "Untitled" || restored.Current().State.Buffer.String() != notebook.Header("Plain Text")+"unsaved 中文😀" {
		t.Fatal("unnamed archived note not recovered")
	}
	for len(restored.Open) > 1 {
		restored.Close(0)
	}
	restored.Close(0)
	if len(restored.Open) != 1 || restored.Current().State.Buffer.String() != notebook.Header("Plain Text") {
		t.Fatal("last-tab close must create a fresh blank note")
	}
}

func TestAtomicSaveFailureAndCorruptRecovery(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	b := notebook.NewBook()
	b.Current().State.Insert("last good")
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	good, _ := os.ReadFile(store.Path())
	// Occupying the destination with a directory makes rename fail, after
	// the temporary file has been written and synced. No temp should leak.
	backup := filepath.Join(store.Dir, "last-good.json")
	if err := os.Rename(store.Path(), backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.Path(), 0700); err != nil {
		t.Fatal(err)
	}
	b.Current().State.Insert(" plus in-memory")
	if err := store.Save(b); err == nil {
		t.Fatal("expected save error")
	}
	if !strings.HasSuffix(b.Current().State.Buffer.String(), " plus in-memory") {
		t.Fatal("failed save lost live content")
	}
	entries, _ := os.ReadDir(store.Dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatal("failed pre-publication save leaked temporary files", e.Name())
		}
	}
	kept, _ := os.ReadFile(backup)
	if string(kept) != string(good) {
		t.Fatal("last good data changed")
	}
	if err := os.Remove(store.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte(`{"version":1,"open":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("corrupt session silently restored")
	}
	corrupt, _ := os.ReadFile(store.Path())
	if string(corrupt) != `{"version":1,"open":` {
		t.Fatal("corrupt source overwritten")
	}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load()
	if err != nil || recovered.Current().State.Buffer.String() != b.Current().State.Buffer.String() {
		t.Fatal("save retry lost live edits", err)
	}
	info, _ := os.Stat(store.Path())
	if info.Mode().Perm() != 0600 {
		t.Fatalf("session permissions: %v", info.Mode())
	}
}

func TestRestoreRejectsInvalidStructureAndPositions(t *testing.T) {
	for _, corruption := range []string{"boundary", "UTF-8 position", "duplicate ID", "active tab", "version"} {
		t.Run(corruption, func(t *testing.T) {
			b := notebook.NewBook()
			b.Current().State.Insert("中文")
			s := b.Snapshot()
			switch corruption {
			case "boundary":
				s.Open[0].Text = "no header"
			case "UTF-8 position":
				s.Open[0].Selections = []editor.Selection{{Anchor: len(notebook.Header("Plain Text")) + 1, Head: len(s.Open[0].Text)}}
			case "duplicate ID":
				s.Closed = append(s.Closed, s.Open[0])
			case "active tab":
				s.Active = 1
			case "version":
				s.Version = 99
			}
			if _, err := notebook.Restore(s); err == nil {
				t.Fatal("invalid session accepted")
			}
		})
	}
}
