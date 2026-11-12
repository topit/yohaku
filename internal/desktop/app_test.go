package desktop_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func click(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
}

// Virtual UI time starts autosave; real worker completion is independent of it.
// Wait for that completion, without replacing autosave with synchronous Flush.
func awaitSave(t *testing.T, tt *ui.Tester, a *desktop.App) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for a.Saving() {
		if time.Now().After(deadline) {
			t.Fatal("background save did not complete")
		}
		tt.Frame()
		runtime.Gosched()
	}
	tt.Frame()
}

func TestImageTextPasteCopySaveFailureAndRestart(t *testing.T) {
	store := notebook.Store{Dir: filepath.Join(t.TempDir(), "session")}
	a := desktop.New(notebook.NewBook(), store)
	tt := ui.NewTester(a.View, 760, 660)
	tt.Type("existing plain control")
	plain := a.Book.Current().State.Buffer.String()
	click(t, tt, "New image & text note")
	tt.Type("before 中文😀\n")
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 12, 7))); err != nil {
		t.Fatal(err)
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, imageData.Bytes()))))
	tt.Command("paste")
	tt.Type("\nafter")
	n := a.Book.Current()
	want := n.Rich.Text().HTML()
	if !strings.Contains(want, "<img ") || !strings.Contains(want, "after") {
		t.Fatal(want)
	}
	tt.Command("selectAll")
	tt.Command("copy")
	html, err := tt.ClipboardData().Read(transfer.HTML)
	if err != nil || string(html) != want {
		t.Fatal("rich copy omitted image", err)
	}
	click(t, tt, "New image & text note")
	tt.Command("paste")
	if a.Book.Current().Rich.Text().HTML() != want {
		t.Fatal("UI paste lost image/order")
	}
	tt.Command("undo")
	if a.Book.Current().Rich.Text().Len() != 0 {
		t.Fatal("UI paste not one undo step")
	}
	tt.Command("redo")
	if err := os.WriteFile(store.Dir, []byte("block saves"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err == nil {
		t.Fatal("save failure hidden")
	}
	if a.Book.Current().Rich.Text().HTML() != want {
		t.Fatal("failed save lost image")
	}
	if err := os.Remove(store.Dir); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyW)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	if a.Book.Current().Rich.Text().HTML() != want {
		t.Fatal("close/reopen lost rich history")
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil || restored.Current().Rich.Text().HTML() != want || restored.Open[0].State.Buffer.String() != plain {
		t.Fatal("restore changed content", err)
	}
	tt.Frame() // Flush cleared the save-error banner outside a UI event.
	area, _ := tt.Find("Image & text editor")
	click(t, tt, "Copy entire rich note")
	tt.Frame()
	if !tt.HasText("Copied image & text") || a.Book.Current().Message != "" {
		t.Fatal("successful copy should show a transient confirmation, not a persistent notice")
	}
	if after, _ := tt.Find("Image & text editor"); after != area || a.Book.Current().Rich.Text().HTML() != want {
		t.Fatalf("copy feedback moved the editor or changed the document: before=%+v after=%+v textEqual=%v", area, after, a.Book.Current().Rich.Text().HTML() == want)
	}
	tt.Advance(3 * time.Second)
	if tt.HasText("Copied image & text") {
		t.Fatal("copy confirmation did not expire")
	}
}

func TestDesktopLifecycleAutosaveAndRestart(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 740)
	tt.Type("a shared thought 中文😀")
	first := a.Book.Current()
	click(t, tt, "+ Block")
	tt.Type("second shared block")
	blocks, err := notebook.Blocks(first.State.Buffer.String())
	if err != nil || len(blocks) != 2 {
		t.Fatal("block button failed", err)
	}
	first.State.Find("shared")
	tt.Frame()
	if count, _ := first.State.FindMatches(); count != 2 {
		t.Fatal("search did not span both blocks")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyR)
	tt.Type("Scratchpad")
	tt.Key(0, ui.KeyEnter)
	if first.Title != "Scratchpad" {
		t.Fatalf("rename: %q", first.Title)
	}
	first.State.Select(blocks[0].Body, blocks[1].Body+6)
	tt.Frame()
	tt.Command("copy")
	if tt.Clipboard() != first.State.Buffer.String()[blocks[0].Body:blocks[1].Body+6] {
		t.Fatalf("cross-block copy failed: clipboard=%q selection=%+v texts=%q", tt.Clipboard(), first.State.Selection(), tt.Texts())
	}
	before, sels := first.State.Buffer.String(), first.State.Selections()
	tt.Compose("中", 1)
	tt.Type("中文")
	if first.State.Buffer.String() != before || !reflect.DeepEqual(first.State.Selections(), sels) {
		t.Fatal("IME crossed protected boundary")
	}
	if _, ok := tt.Find(notebook.Rejected); !ok {
		t.Fatal("no boundary feedback")
	}
	click(t, tt, "Dismiss")
	click(t, tt, "New note")
	tt.Type("independent")
	second := a.Book.Current()
	click(t, tt, "Scratchpad")
	if a.Book.Current() != first || !reflect.DeepEqual(first.State.Selections(), sels) {
		t.Fatal("switch lost selection")
	}
	click(t, tt, "Close Scratchpad")
	if a.Book.Current() != second {
		t.Fatal("closing active tab selected wrong note")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	if a.Book.Current() != first {
		t.Fatal("reopen lost original buffer")
	}
	first.State.Run(editor.Undo)
	if strings.Contains(first.State.Buffer.String(), "second shared block") {
		t.Fatal("reopen lost undo")
	}
	first.State.Run(editor.Redo)
	tt.Frame()
	tt.Advance(time.Second)
	awaitSave(t, tt, a)
	if a.SaveError != "" {
		t.Fatal(a.SaveError)
	}
	if _, ok := tt.Find("Saved locally"); !ok {
		t.Fatal("save status stale")
	}
	restored, err := store.Load()
	if err != nil || !reflect.DeepEqual(restored.Snapshot(), a.Book.Snapshot()) {
		t.Fatal("autosave/restart mismatch", err)
	}
}

func TestBackgroundSaveIncludesEditsAndTabChangesBeforeClose(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 740)
	tt.Type("first version")
	a.RequestSave()
	// No UI poll has acknowledged the first job. Several newer requests must
	// coalesce, without letting its completion mark these edits as saved.
	tt.Type(" + latest 中文😀")
	a.RequestSave()
	a.Book.NewRich()
	a.Book.Current().Rich.Insert(richtext.NewText("rich latest"))
	a.RequestSave()
	a.Book.Rename("renamed during save")
	a.Book.Close(0)
	resumed := 0
	resume := func() { resumed++ }
	if a.PrepareClose(resume) {
		t.Fatal("close bypassed pending writes")
	}
	// An edit after the close request must be included as well.
	a.Book.Current().Rich.Insert(richtext.NewText(" after close request"))
	awaitSave(t, tt, a)
	if resumed != 1 || !a.PrepareClose(resume) || a.SaveError != "" {
		t.Fatalf("close gate: resumed=%d error=%q", resumed, a.SaveError)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(a.Book.Snapshot())
	got, _ := json.Marshal(loaded.Snapshot())
	if !bytes.Equal(want, got) {
		t.Fatalf("latest content/tab/archive state was not durable before close:\nloaded=%s\nlive=%s", got, want)
	}
}

func TestReadingDoesNotTriggerAutosave(t *testing.T) {
	for _, rich := range []bool{false, true} {
		name := "blocks"
		if rich {
			name = "image-text"
		}
		t.Run(name, func(t *testing.T) {
			b := notebook.NewBook()
			if rich {
				b.NewRich()
			}
			a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
			defer a.Close()
			tt := ui.NewTester(a.View, 1080, 740)
			tt.Type(strings.Repeat("a thought to read\n", 80))
			tt.Advance(time.Second)
			awaitSave(t, tt, a)
			before, err := os.ReadFile(a.Store.Path())
			if err != nil {
				t.Fatal(err)
			}
			n := b.Current()
			if rich {
				n.Rich.SetSelection(2, 7)
			} else {
				n.State.Select(len(notebook.Header("Plain Text"))+2, len(notebook.Header("Plain Text"))+7)
			}
			tt.Frame()
			tt.Scroll(500, 350, 0, 300)
			tt.Advance(time.Millisecond)
			if tt.HasText("Saving…") || !tt.HasText("Saved locally") {
				t.Fatal("reading changed the save indicator", tt.Texts())
			}
			tt.Advance(time.Second)
			after, err := os.ReadFile(a.Store.Path())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("reading rewrote the session", err)
			}
			want, err := json.Marshal(b.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			if string(want) == mustLoadSnapshotJSON(t, a.Store) {
				t.Fatal("reading did not change the live selection/scroll")
			}
			// The close/quit gate still persists the latest reading position.
			if err := a.Flush(); err != nil {
				t.Fatal(err)
			}
			if string(want) != mustLoadSnapshotJSON(t, a.Store) {
				t.Fatal("explicit save lost the reading position")
			}
			tt.Type("updated")
			tt.Advance(time.Millisecond)
			if !tt.HasText("Saving…") {
				t.Fatal("content edits no longer trigger autosave")
			}
			tt.Advance(time.Second)
			awaitSave(t, tt, a)
			want, err = json.Marshal(b.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != mustLoadSnapshotJSON(t, a.Store) {
				t.Fatal("edited content was not autosaved")
			}
		})
	}
}

func mustLoadSnapshotJSON(t *testing.T, store notebook.Store) string {
	t.Helper()
	b, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(b.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSaveFailureKeepsMemoryAndRetries(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 740)
	tt.Type("last good")
	tt.Advance(time.Second)
	awaitSave(t, tt, a)
	good, _ := os.ReadFile(store.Path())
	if err := os.Rename(store.Dir, store.Dir+"-backup"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(store.Dir + "-backup") })
	if err := os.WriteFile(store.Dir, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	tt.Type(" still in memory")
	tt.Advance(time.Millisecond) // Run After(0)'s timer, without reaching autosave.
	if _, ok := tt.Find("Saving…"); !ok {
		t.Fatal("pending edits still show Saved locally")
	}
	tt.Advance(time.Second)
	awaitSave(t, tt, a)
	if a.SaveError == "" {
		t.Fatal("save failure not recorded")
	}
	if _, ok := tt.Find("Retry save"); !ok {
		t.Fatal("failure not visibly actionable")
	}
	if err := a.Flush(); err == nil {
		t.Fatal("close gate allowed failed save")
	}
	resumed := false
	if a.PrepareClose(func() { resumed = true }) {
		t.Fatal("native close gate bypassed failed storage")
	}
	awaitSave(t, tt, a)
	if resumed || a.SaveError == "" {
		t.Fatal("failed asynchronous close discarded the error or resumed closing")
	}
	a.OpenSettings()
	tt.Frame()
	if !tt.HasText(a.SaveError) {
		t.Fatal("background failure hidden by settings")
	}
	tt.Key(0, ui.KeyEscape)
	if !strings.HasSuffix(a.Book.Current().State.Buffer.String(), " still in memory") {
		t.Fatal("save failure lost edits")
	}
	kept, _ := os.ReadFile(store.Dir + "-backup/session.json")
	if string(kept) != string(good) {
		t.Fatal("last good save changed")
	}
	if err := os.Remove(store.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(store.Dir+"-backup", store.Dir); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Retry save")
	awaitSave(t, tt, a)
	if a.SaveError != "" {
		t.Fatal("retry failed", a.SaveError)
	}
	restored, err := store.Load()
	if err != nil || !strings.HasSuffix(restored.Current().State.Buffer.String(), " still in memory") {
		t.Fatal("retry did not persist pending edit", err)
	}
}

func TestInsertBlockBeforeShortcut(t *testing.T) {
	text := notebook.Header("Go") + "first 中文😀\n" + notebook.Header("Python") + "second\n" + notebook.Header("JSON") + "{}"
	for _, index := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			n, err := notebook.NewNote("Before", text)
			if err != nil {
				t.Fatal(err)
			}
			b := &notebook.Book{Open: []*notebook.Note{n}}
			a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
			tt := ui.NewTester(a.View, 1080, 740)
			blocks, _ := notebook.Blocks(text)
			at := blocks[index].Start
			n.State.Select(blocks[index].End, blocks[index].End)
			tt.Key(ui.Alt, ui.KeyEnter)
			want := text[:at] + notebook.Header("Plain Text") + "\n" + text[at:]
			if n.State.Buffer.String() != want || n.State.Cursor() != at+len(notebook.Header("Plain Text")) {
				t.Fatal("shortcut did not insert/focus an empty block before the current one")
			}
			tt.Type("inserted")
			if n.Message != "" || n.State.Buffer.String() != text[:at]+notebook.Header("Plain Text")+"inserted\n"+text[at:] {
				t.Fatal("new caret was not in the inserted body")
			}
			tt.Command("undo")
			if n.State.Buffer.String() != want {
				t.Fatal("typing was grouped with insertion")
			}
			tt.Command("undo")
			if n.State.Buffer.String() != text {
				t.Fatal("insertion did not undo in one step")
			}
			tt.Command("redo")
			if n.State.Buffer.String() != want {
				t.Fatal("insertion redo changed surrounding blocks")
			}
		})
	}
}

func TestEmptyBlockBackspace(t *testing.T) {
	for _, mode := range []string{"middle", "last", "native-delete", "first", "only", "whitespace", "multiple-carets", "cross-block-selection"} {
		t.Run(mode, func(t *testing.T) {
			text := notebook.Header("Go") + "previous 中文😀\n" + notebook.Header("Python") + "\n" + notebook.Header("JSON") + "{}"
			index := 1
			switch mode {
			case "last":
				text = notebook.Header("Go") + "previous 中文😀\n" + notebook.Header("Python")
			case "first":
				text, index = notebook.Header("Python")+"\n"+notebook.Header("Go")+"next", 0
			case "only":
				text, index = notebook.Header("Python"), 0
			case "whitespace":
				text = notebook.Header("Go") + "previous 中文😀\n" + notebook.Header("Python") + " \n" + notebook.Header("JSON") + "{}"
			}
			n, err := notebook.NewNote("Empty", text)
			if err != nil {
				t.Fatal(err)
			}
			a := desktop.New(&notebook.Book{Open: []*notebook.Note{n}}, notebook.Store{Dir: t.TempDir()})
			tt := ui.NewTester(a.View, 1080, 740)
			blocks, _ := notebook.Blocks(text)
			n.State.Select(blocks[index].Body, blocks[index].Body)
			if mode == "multiple-carets" {
				n.State.SetSelections(editor.Selection{Anchor: blocks[0].End, Head: blocks[0].End}, editor.Selection{Anchor: blocks[1].Body, Head: blocks[1].Body})
			} else if mode == "cross-block-selection" {
				n.State.Select(blocks[0].End-1, blocks[1].Body)
			}
			before := n.State.Selections()
			if mode == "native-delete" {
				tt.Command("delete")
			} else {
				tt.Key(0, ui.KeyBackspace)
			}
			if mode == "middle" || mode == "last" || mode == "native-delete" {
				want := text[:blocks[1].Start-1] + text[blocks[1].End:]
				if n.State.Buffer.String() != want || n.State.Cursor() != blocks[0].End || n.Message != "" {
					t.Fatal("empty block was not removed with caret at previous body's end")
				}
				tt.Command("undo")
				if n.State.Buffer.String() != text {
					t.Fatal("empty block deletion not independently undoable")
				}
				tt.Command("redo")
				if n.State.Buffer.String() != want {
					t.Fatal("empty block deletion redo incorrect")
				}
			} else if n.State.Buffer.String() != text || !reflect.DeepEqual(n.State.Selections(), before) {
				t.Fatal("backspace unexpectedly removed content, structure or selections")
			}
		})
	}
}

func TestSearchReplacementDoesNotRunBlockShortcut(t *testing.T) {
	b := notebook.NewBook()
	b.Current().State.Insert("shared Text\n")
	b.Current().AddBlock()
	b.Current().State.Insert("shared")
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	tt := ui.NewTester(a.View, 1080, 740)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("shared")
	click(t, tt, "Replace")
	focusReplacement := func() {
		bar, ok := tt.Find("Find")
		all, visible := tt.Find("Replace All")
		if !ok || !visible {
			t.Fatal("replacement row not visible")
		}
		// The default bar names its toggle, field and button "Replace".
		// Click the field at the left of its replacement row.
		tt.ClickAt(bar.X+20, all.Y+all.H/2)
	}
	focusReplacement()
	tt.Type("changed")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Key(ui.Alt, ui.KeyEnter)
	if blocks, err := notebook.Blocks(b.Current().State.Buffer.String()); err != nil || len(blocks) != 2 {
		t.Fatal("replacement field ran add-block shortcut")
	}
	click(t, tt, "Replace All")
	want := notebook.Header("Plain Text") + "changed Text\n\n" + notebook.Header("Plain Text") + "changed"
	if b.Current().State.Buffer.String() != want {
		t.Fatalf("replace all failed: %q", b.Current().State.Buffer.String())
	}
	// Searching structural and body text together must reject everything,
	// not partially replace only body matches.
	tt.Key(0, ui.KeyEscape)
	b.Current().State.Find("Text")
	tt.Frame()
	// Replacing remains expanded when reopening this search.
	focusReplacement()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("unsafe")
	before, sels := b.Current().State.Buffer.String(), b.Current().State.Selections()
	click(t, tt, "Replace All")
	if b.Current().State.Buffer.String() != before || !reflect.DeepEqual(b.Current().State.Selections(), sels) {
		t.Fatal("replace all damaged structure or moved selection")
	}
}

func TestScrollSelectionRestoreAndIndependentHistory(t *testing.T) {
	b := notebook.NewBook()
	n := b.Current()
	// A note shorter than the viewport must still scroll its last line up.
	n.State.Insert("first thought\nsecond thought\nlast thought")
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	tt := ui.NewTester(a.View, 1080, 740)
	n.State.Select(len(notebook.Header("Plain Text"))+2, len(notebook.Header("Plain Text"))+7)
	tt.Frame()
	tt.Scroll(500, 350, 0, 10000)
	tt.Frame()
	scroll, sel := n.State.ScrollY(), n.State.Selection()
	if scroll <= 0 {
		t.Fatal("not scrolled")
	}
	if top := n.State.LineTop(n.State.Buffer.LineCount()-1) - scroll; top < -1 || top > 1 {
		t.Fatalf("last line must reach the top of the editing viewport, got y=%v", top)
	}
	click(t, tt, "New note")
	tt.Type("other")
	click(t, tt, "Untitled")
	if n.State.ScrollY() != scroll || n.State.Selection() != sel {
		t.Fatal("tab switch lost position")
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	restored, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	a2 := desktop.New(restored, a.Store)
	tt2 := ui.NewTester(a2.View, 1080, 740)
	tt2.Frame()
	if restored.Current().State.ScrollY() != scroll || restored.Current().State.Selection() != sel {
		t.Fatalf("restart position: scroll=%v expected=%v selection=%+v", restored.Current().State.ScrollY(), scroll, restored.Current().State.Selection())
	}
}

func TestTabContextActionsPreserveNotesAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		action string
		keep   []int
		active int
	}{
		{"Rename tab", []int{0, 1, 2, 3, 4}, 1},
		{"Close tab", []int{0, 2, 3, 4}, 3},
		{"Close other tabs", []int{1}, 1},
		{"Close tabs to the right", []int{0, 1}, 1},
	} {
		t.Run(tc.action, func(t *testing.T) {
			b := notebook.NewBook()
			var notes []*notebook.Note
			for i, name := range []string{"Alpha", "Bravo", "Images", "Delta", "Echo"} {
				if i == 2 {
					b.NewRich()
				} else if i > 0 {
					b.New()
				}
				b.Rename(name)
				n := b.Current()
				if n.Rich != nil {
					n.Rich.InsertText("Rich 正文")
				} else {
					n.State.Insert(name + " 正文")
				}
				notes = append(notes, n)
			}
			b.Switch(3)
			store := notebook.Store{Dir: t.TempDir()}
			a := desktop.New(b, store)
			defer a.Close()
			tt := ui.NewTester(a.View, 1080, 660)
			if err := tt.RightClick("Bravo"); err != nil {
				t.Fatal(err)
			}
			if b.Current().ID != notes[3].ID {
				t.Fatal("opening a background tab menu switched the active note")
			}
			if err := tt.ChooseMenuItem(tc.action); err != nil {
				t.Fatal(err, tt.Menu())
			}
			tt.Frame()
			if tc.action == "Rename tab" {
				tt.Type("Renamed 中文")
				tt.Key(0, ui.KeyEnter)
				if notes[1].Title != "Renamed 中文" || notes[3].Title != "Delta" {
					t.Fatal("renamed the active tab instead of the clicked tab")
				}
			}
			if len(b.Open) != len(tc.keep) || len(b.Closed) != len(notes)-len(tc.keep) || b.Current().ID != notes[tc.active].ID {
				t.Fatal("wrong survivors, active note or archive count", b.Snapshot())
			}
			for i, index := range tc.keep {
				if b.Open[i].ID != notes[index].ID {
					t.Fatal("changed survivor order or identity")
				}
			}
			if err := a.Flush(); err != nil {
				t.Fatal(err)
			}
			restored, err := store.Load()
			if err != nil {
				t.Fatal("bulk close lost note data on restart", err)
			}
			want, _ := json.Marshal(b.Snapshot())
			got, _ := json.Marshal(restored.Snapshot())
			if !bytes.Equal(got, want) {
				t.Fatalf("restart changed saved note data:\n%s\nwant:\n%s", got, want)
			}
			for len(b.Closed) > 0 {
				tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
			}
			if len(b.Open) != len(notes) {
				t.Fatal("could not reopen all archived notes")
			}
			// Batch close/reopen must preserve both editor models' undo.
			notes[3].State.Run(editor.Undo)
			if notes[3].State.Buffer.String() != notebook.Header("Plain Text") {
				t.Fatal("plain-note undo lost during a tab action")
			}
			notes[2].Rich.Undo()
			if notes[2].Rich.Text().Len() != 0 {
				t.Fatal("rich-note undo lost during a tab action")
			}
			b.Switch(len(b.Open) - 1)
			tt.Frame()
			if err := tt.RightClick(b.Current().Title); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Close tabs to the right"); err == nil {
				t.Fatal("last tab allowed close-right")
			}
		})
	}
	t.Run("only tab", func(t *testing.T) {
		b := notebook.NewBook()
		n := b.Current()
		n.State.Insert("Keep this draft")
		a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
		defer a.Close()
		tt := ui.NewTester(a.View, 760, 480)
		if err := tt.RightClick(n.Title); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"Close other tabs", "Close tabs to the right"} {
			if err := tt.ChooseMenuItem(action); err == nil {
				t.Fatal("allowed a no-op bulk action on the only tab", action)
			}
		}
		if err := tt.ChooseMenuItem("Close tab"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if len(b.Open) != 1 || b.Current().ID == n.ID || len(b.Closed) != 1 {
			t.Fatal("closing the last tab did not leave a blank note and archive")
		}
		tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
		if b.Current().ID != n.ID || !strings.HasSuffix(b.Current().State.Buffer.String(), "Keep this draft") {
			t.Fatal("closing the last tab lost its draft")
		}
	})
}

func TestCompactTabsRenameRevealAndLanguageMenu(t *testing.T) {
	b := notebook.NewBook()
	for _, title := range []string{"First thought", "Reading notes", "Long running ideas", "Design references", "中文随笔", "Last thought"} {
		b.Rename(title)
		b.New()
	}
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	tt := ui.NewTester(a.View, 760, 600)
	tt.Advance(time.Millisecond)
	visibleClose := func() {
		t.Helper()
		track, ok := tt.Find("Tabs")
		close, found := tt.Find("Close " + b.Current().Title)
		if !ok || !found || close.X < track.X || close.X+close.W > track.X+track.W+1 {
			t.Fatalf("active tab's close button is clipped: tab=%+v track=%+v", close, track)
		}
	}
	visibleClose()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyR)
	tt.Type("An intentionally long title for the active note")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Millisecond)
	visibleClose()
	before := b.Current().Title
	tt.Key(ui.Cmd|ui.Shift, ui.KeyR)
	tt.Type("cancelled name")
	tt.Key(0, ui.KeyEscape)
	tt.Type("body after cancelling rename")
	if b.Current().Title != before || !strings.HasSuffix(b.Current().State.Buffer.String(), "body after cancelling rename") {
		t.Fatal("inline rename cancellation lost name or editor focus")
	}
	click(t, tt, "Block language")
	click(t, tt, "Go")
	blocks, _ := notebook.Blocks(b.Current().State.Buffer.String())
	if blocks[0].Language != "Go" {
		t.Fatal("language menu did not change the block")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	blocks, _ = notebook.Blocks(b.Current().State.Buffer.String())
	if blocks[0].Language != "Plain Text" {
		t.Fatal("language menu did not return focus for undo")
	}
	click(t, tt, "Block language")
	tt.Key(0, ui.KeyEnd)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	blocks, _ = notebook.Blocks(b.Current().State.Buffer.String())
	if blocks[0].Language != "Zig" {
		t.Fatal("expanded language menu did not reach its last entry")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	// More keeps low-frequency actions discoverable without changing the
	// original cursor. Both structure operations remain one undo step.
	text := b.Current().State.Buffer.String()
	click(t, tt, "Block actions")
	click(t, tt, "Split block at cursor")
	if split, _ := notebook.Blocks(b.Current().State.Buffer.String()); len(split) != 2 {
		t.Fatal("split menu did not operate on the original note")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if b.Current().State.Buffer.String() != text {
		t.Fatal("split menu lost editor focus or was not one undo step")
	}
	click(t, tt, "Block actions")
	click(t, tt, "Delete current block…")
	if !tt.HasText("Delete this block") || b.Current().State.Buffer.String() != text {
		t.Fatal("delete menu skipped confirmation")
	}
	tt.Key(0, ui.KeyEscape)
	if b.Current().State.Buffer.String() != text {
		t.Fatal("cancelled deletion changed the document")
	}
	click(t, tt, "Block actions")
	click(t, tt, "Delete current block…")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if b.Current().State.Buffer.String() != notebook.Header("Plain Text") {
		t.Fatal("confirmed deletion did not clear the sole block")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if b.Current().State.Buffer.String() != text {
		t.Fatal("delete menu lost editor focus or could not be undone")
	}
	click(t, tt, "Close "+b.Current().Title)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	tt.Advance(time.Millisecond)
	visibleClose()
	b.Switch(0)
	tt.Frame()
	tt.Advance(time.Millisecond)
	visibleClose()
}

func TestEditorSurfaceFitsWindowAfterResize(t *testing.T) {
	a := desktop.New(notebook.NewBook(), notebook.Store{Dir: t.TempDir()})
	tt := ui.NewTester(a.View, 1080, 660)
	for _, width := range []int{1080, 760, 1280} {
		tt.SetSize(width, 660)
		card, ok := tt.Find("Editor surface")
		if !ok || card.X < 6 || float32(width)-(card.X+card.W) < 6 {
			t.Fatalf("editor surface clips its rounded edge at width %d: %+v", width, card)
		}
	}
}

func TestPaperFooterFitsNarrowWindow(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans"} {
		t.Run(locale, func(t *testing.T) {
			store := notebook.Store{Dir: t.TempDir()}
			if err := os.WriteFile(filepath.Join(store.Dir, "preferences.json"), []byte(`{"language":"`+locale+`"}`), 0600); err != nil {
				t.Fatal(err)
			}
			book := notebook.NewBook()
			a := desktop.New(book, store)
			defer a.Close()
			tt := ui.NewTester(a.View, 760, 480)
			for _, language := range []string{"Plain Text", "Markdown", "Math", "JavaScript"} {
				book.Current().SetLanguage(language)
				tt.Frame()
				previousEnd := float32(0)
				labels := []string{"Block language", "+ Block", a.T("Block actions")}
				if language == "Math" {
					labels = []string{"Block language", "Copy result", "+ Block", a.T("Block actions")}
				}
				for _, label := range labels {
					r, ok := tt.Find(label)
					if !ok || r.W < 18 || r.X < previousEnd || r.X+r.W > 752 || r.Y < 440 || r.Y+r.H > 472 {
						t.Fatalf("%s footer control %q clipped or overlapping: %+v", language, label, r)
					}
					previousEnd = r.X + r.W
				}
				click(t, tt, a.T("Block actions"))
				for _, item := range []string{"Insert block before", "Split block at cursor", "Delete current block…"} {
					r, ok := tt.Find(a.T(item))
					if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 760 || r.Y+r.H > 440 {
						t.Fatalf("footer menu item %q outside visible area: %+v", item, r)
					}
				}
				tt.Key(0, ui.KeyEscape)
			}
		})
	}
}

func TestBlockPresentationAlternatesWithoutChangingDocument(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			b := notebook.NewBook()
			n := b.Current()
			n.State.Insert("first 中文😀 " + strings.Repeat("a wrapping thought ", 8))
			n.AddBlock()
			n.SetLanguage("Plain Text")
			n.State.Insert("second text block")
			n.AddBlock()
			n.SetLanguage("JSON")
			n.State.Insert(`{"shared": true}`)
			n.AddBlock()
			n.SetLanguage("Markdown")
			n.State.Insert("# Markdown block")
			n.AddBlock()
			n.SetLanguage("Math")
			n.State.Insert("12 * 8")
			n.AddBlock()
			n.SetLanguage("Math")
			n.State.Insert("14.9 / 238")
			before := n.State.Buffer.String()
			store := notebook.Store{Dir: t.TempDir()}
			if err := os.WriteFile(filepath.Join(store.Dir, "preferences.json"), []byte(`{"mode":"`+mode+`"}`), 0600); err != nil {
				t.Fatal(err)
			}
			a := desktop.New(b, store)
			tt := ui.NewTester(a.View, 760, 660)
			area, ok := tt.Find("Single-buffer block editor")
			if !ok {
				t.Fatal("missing editor")
			}
			blocks, _ := notebook.Blocks(before)
			img := tt.Image()
			scale := float32(img.Bounds().Dx()) / 760
			pixel := func(y float32) [3]uint8 {
				// Sample the text canvas, not the newly visible number gutter.
				p := img.RGBAAt(int((area.X+area.W-24)*scale), int((area.Y+y)*scale))
				return [3]uint8{p.R, p.G, p.B}
			}
			snapshot := n.State.Buffer.Snapshot()
			bodyColor := func(index int) [3]uint8 {
				return pixel(float32(n.State.LineTop(snapshot.LineOf(blocks[index].Body))) + 3)
			}
			first, second, third := bodyColor(0), bodyColor(1), bodyColor(2)
			if first == second {
				t.Fatal("adjacent text blocks have the same background")
			}
			for i := range blocks {
				if got, want := bodyColor(i), [][3]uint8{first, second}[i%2]; got != want {
					t.Fatalf("block %d background = %v, want %v", i+1, got, want)
				}
			}
			if pixel(area.H-20) != first {
				t.Fatal("canvas after the final Math block is not paper")
			}
			for i := 1; i < len(blocks); i++ {
				bodyLine := snapshot.LineOf(blocks[i].Body)
				ruleY := float32(n.State.LineTop(bodyLine-1)+n.State.LineTop(bodyLine)) / 2
				if pixel(ruleY-3) != bodyColor(i-1) || pixel(ruleY+3) != bodyColor(i) {
					t.Fatalf("block %d background changes before or after its divider", i+1)
				}
			}
			// The first structural row is blank, not a visible protocol title.
			for y := 1; y < int(n.State.LineTop(1))-1; y++ {
				for x := 12; x < 250; x++ {
					p := img.RGBAAt(int((area.X+float32(x))*scale), int((area.Y+float32(y))*scale))
					if [3]uint8{p.R, p.G, p.B} != first {
						t.Fatal("a structural title is still visible in the editor")
					}
				}
			}
			if n.State.Buffer.String() != before {
				t.Fatal("presentation changed document text")
			}
			// Appending never recolors existing bodies, gutter or trailing paper.
			gutterColor := func(y float32) [3]uint8 {
				p := img.RGBAAt(int((area.X+2)*scale), int((area.Y+y)*scale))
				return [3]uint8{p.R, p.G, p.B}
			}
			gutter := gutterColor(area.H - 20)
			for range 3 {
				n.AddBlock()
				n.SetLanguage("Go")
				tt.Frame()
				img = tt.Image()
				for _, y := range []float32{3, 60, area.H - 20} {
					if got := gutterColor(y); got != gutter {
						t.Fatalf("appending a block recolored the gutter: %v → %v", gutter, got)
					}
				}
				if bodyColor(0) != first || bodyColor(1) != second || bodyColor(2) != third {
					t.Fatal("appending a block recolored an existing body")
				}
				current, _ := notebook.Blocks(n.State.Buffer.String())
				last := current[len(current)-1]
				lastColor := pixel(float32(n.State.LineTop(n.State.Buffer.Snapshot().LineOf(last.Body))) + 3)
				if lastColor != [][3]uint8{first, second}[(len(current)-1)%2] || pixel(area.H-20) != first {
					t.Fatal("new code block or trailing paper has the wrong color")
				}
			}
			for range 6 {
				n.State.Run(editor.Undo)
			}
			tt.Frame()
			img = tt.Image()
			if n.State.Buffer.String() != before || gutterColor(area.H-20) != gutter || pixel(area.H-20) != first {
				t.Fatal("undo did not restore document and trailing canvas without recoloring gutter")
			}
			checkAfterRemoval := func(t *testing.T, wantCount int) {
				t.Helper()
				tt.Frame()
				img = tt.Image()
				snapshot = n.State.Buffer.Snapshot()
				blocks, _ = notebook.Blocks(n.State.Buffer.String())
				if len(blocks) != wantCount {
					t.Fatalf("block count = %d, want %d", len(blocks), wantCount)
				}
				for i, block := range blocks {
					if got, want := bodyColor(i), [][3]uint8{first, second}[i%2]; got != want {
						t.Fatalf("block %d after removal = %v, want %v", i+1, got, want)
					}
					if n.State.LineNumber(snapshot.LineOf(block.Body)) != 1 {
						t.Fatalf("block %d line numbers did not restart", i+1)
					}
				}
				if pixel(area.H-20) != first || gutterColor(area.H-20) != gutter {
					t.Fatal("removal recolored trailing paper or gutter")
				}
			}
			// Delete each original position, including both sides of same-type
			// pairs, then exercise both undo and redo on the rendered palette.
			for index := range 6 {
				t.Run("remove-"+strconv.Itoa(index+1), func(t *testing.T) {
					n.State.Select(blocks[index].Body, blocks[index].Body)
					n.RemoveBlock()
					checkAfterRemoval(t, 5)
					n.State.Run(editor.Undo)
					checkAfterRemoval(t, 6)
					if n.State.Buffer.String() != before {
						t.Fatal("undo removal did not restore document")
					}
					n.State.Run(editor.Redo)
					checkAfterRemoval(t, 5)
					n.State.Run(editor.Undo)
					checkAfterRemoval(t, 6)
				})
			}
			for count := 6; count > 1; count-- {
				n.State.Select(blocks[0].Body, blocks[0].Body)
				n.RemoveBlock()
				checkAfterRemoval(t, count-1)
			}
			n.RemoveBlock() // The final block becomes an empty paper block.
			checkAfterRemoval(t, 1)
		})
	}
}

func TestDefaultHeynoteImportRequiresConsentAndDoesNotRepeat(t *testing.T) {
	for _, allow := range []bool{false, true} {
		name := "decline"
		if allow {
			name = "allow"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, "Library", "Application Support", "Heynote", "notes")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "draft.txt")
			if err := os.WriteFile(path, []byte("not a Heynote note yet"), 0600); err != nil {
				t.Fatal(err)
			}
			store := notebook.Store{Dir: t.TempDir()}
			a := desktop.New(notebook.NewBook(), store)
			defer a.Close()
			tt := ui.NewTester(a.View, 760, 480)
			tt.Type("existing 中文 draft")
			before := a.Book.Snapshot()
			a.OfferHeynoteImport()
			tt.Frame()
			tt.Key(ui.Cmd, ui.KeyN)
			if !tt.HasText("Bring your Heynote notes along") || !reflect.DeepEqual(before, a.Book.Snapshot()) || a.Book.Current().Message != "" {
				t.Fatal("discovery read/imported notes or modal leaked a shortcut", tt.Texts())
			}
			// The library can change while consent is pending. Import the actual
			// authorized contents, not an eagerly loaded pre-consent snapshot.
			source := "{\"name\":\"Imported 中文\"}\n∞∞∞text\nnew contents 😀\n∞∞∞python\nprint(42)"
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			want := 1
			if allow {
				click(t, tt, "Allow & import")
				want = 2
			} else {
				tt.Key(0, ui.KeyEscape)
			}
			tt.Frame()
			if len(a.Book.Open) != want || !a.Book.HeynoteImportHandled || a.Book.Open[0].State.Buffer.String() != before.Open[0].Text {
				t.Fatal("consent result changed the wrong notes")
			}
			if allow && (a.Book.Current().Title != "Imported 中文" || a.Book.Current().State.Buffer.String() != notebook.Header("Plain Text")+"new contents 😀\n"+notebook.Header("Python")+"print(42)") {
				t.Fatal("automatic import lost title, language or text")
			}
			awaitSave(t, tt, a)
			restored, err := store.Load()
			if err != nil || !restored.HeynoteImportHandled || len(restored.Open) != want {
				t.Fatal("consent and notes did not persist together", err)
			}
			after := desktop.New(restored, store)
			defer after.Close()
			after.OfferHeynoteImport()
			rt := ui.NewTester(after.View, 1080, 720)
			if rt.HasText("Allow & import") || len(after.Book.Open) != want {
				t.Fatal("restart prompted or imported again")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != source {
				t.Fatal("Heynote source modified", err)
			}
			// Declining doesn't revoke the explicit settings entry. Cancelling
			// the second dialog returns to settings rather than discarding it.
			after.OpenSettings()
			rt.Frame()
			click(t, rt, "Data")
			rt.Scroll(500, 300, 0, 10000)
			click(t, rt, "Import from default folder…")
			rt.Frame()
			if !rt.HasText("Allow & import") {
				t.Fatal("explicit reauthorization unavailable")
			}
			click(t, rt, "Don't import")
			rt.Frame()
			if !rt.HasText("Save settings") || len(after.Book.Open) != want {
				t.Fatal("declining discarded settings or imported notes")
			}
		})
	}
}

func TestDefaultHeynoteImportFailureAndAtomicCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "Application Support", "Heynote", "notes")
	store := notebook.Store{Dir: filepath.Join(t.TempDir(), "session")}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	a.OfferHeynoteImport() // Missing default library must not interrupt launch.
	tt := ui.NewTester(a.View, 1080, 720)
	if tt.HasText("Allow & import") || a.Book.HeynoteImportHandled {
		t.Fatal("missing directory offered or marked handled")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	a.OfferHeynoteImport()
	tt.Frame()
	click(t, tt, "Allow & import")
	if !tt.HasText("Heynote import failed — nothing imported:") || a.Book.HeynoteImportHandled || len(a.Book.Open) != 1 {
		t.Fatal("failed import was completed or hidden", tt.Texts())
	}
	if err := os.WriteFile(path, []byte("∞∞∞text\nretry 中文"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(store.Dir, store.Dir+"-kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Dir, []byte("block saves"), 0600); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Allow & import")
	tt.Frame()
	awaitSave(t, tt, a)
	if len(a.Book.Open) != 2 || a.SaveError == "" || !strings.HasSuffix(a.Book.Current().State.Buffer.String(), "retry 中文") {
		t.Fatal("save failure lost the authorized import")
	}
	old, err := (notebook.Store{Dir: store.Dir + "-kept"}).Load()
	if err != nil || old.HeynoteImportHandled || len(old.Open) != 1 {
		t.Fatal("completion marker persisted separately from notes", err)
	}
	if err := os.Remove(store.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(store.Dir+"-kept", store.Dir); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil || !restored.HeynoteImportHandled || len(restored.Open) != 2 || restored.Current().State.Buffer.String() != notebook.Header("Plain Text")+"retry 中文" {
		t.Fatal("repaired save lost import/completion", err)
	}
}

func TestHeynoteImportFeedbackAutosaveAndRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calculations.txt")
	if err := os.WriteFile(path, []byte("{\"formatVersion\":\"2.0.0\",\"name\":\"Budget\"}\n∞∞∞math\nprice = 19\nprice * 3"), 0600); err != nil {
		t.Fatal(err)
	}
	store := notebook.Store{Dir: t.TempDir()}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 660)
	tt.Type("existing draft")
	a.ImportHeynote([]string{path})
	tt.Frame()
	if len(a.Book.Open) != 2 || a.Book.Current().Title != "Budget" || !strings.Contains(a.Book.Current().Message, "Imported 1") {
		t.Fatal("missing imported tab or feedback")
	}
	if _, ok := tt.Find("Math"); !ok {
		t.Fatal("math language missing")
	}
	tt.Advance(time.Second)
	awaitSave(t, tt, a)
	restored, err := store.Load()
	if err != nil || len(restored.Open) != 2 || restored.Current().State.Buffer.String() != "⟦yohaku:Math⟧\nprice = 19\nprice * 3" {
		t.Fatal("import not autosaved", err)
	}
	before := a.Book.Snapshot()
	a.ImportHeynote(nil)
	if !reflect.DeepEqual(before, a.Book.Snapshot()) {
		t.Fatal("cancel changed data")
	}
	a.ImportHeynote([]string{filepath.Join(dir, "missing.txt")})
	tt.Frame()
	if !strings.Contains(a.Book.Current().Message, "nothing imported") || a.SaveError != "" || !reflect.DeepEqual(before, a.Book.Snapshot()) {
		t.Fatal("import error changes data or is confused with save failure")
	}
}

func TestMathEditingCopyUndoAndRestart(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	b := notebook.NewBook()
	b.Current().SetLanguage("Math")
	a := desktop.New(b, store)
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 660)
	tt.Type("price = 19\nquantity = 3\nprice x quantity")
	n := b.Current()
	awaitResult := func(tt *ui.Tester, want string) {
		t.Helper()
		tt.SetClipboard("waiting for a fresh result")
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			tt.Advance(200 * time.Millisecond)
			tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
			if tt.Clipboard() == want {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatalf("want copied result %q, got %q; UI %v", want, tt.Clipboard(), tt.Texts())
	}
	beforeResult, focused := tt.TextCaret()
	if !focused {
		t.Fatal("missing insertion point before calculation")
	}
	awaitResult(tt, "57")
	if afterResult, ok := tt.TextCaret(); !ok || afterResult != beforeResult {
		t.Fatalf("Math result moved the source caret: before=%+v after=%+v", beforeResult, afterResult)
	}
	text := n.State.Buffer.String()
	click(t, tt, "Copy result")
	if tt.Clipboard() != "57" || n.State.Buffer.String() != text {
		t.Fatal("result button changed source or copied wrong value")
	}
	// Make a new result and immediately try copying; stale 57 must not win.
	n.State.Select(len(text), len(text))
	tt.Type(" + 5")
	tt.SetClipboard("stale guard")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if tt.Clipboard() != "stale guard" {
		t.Fatal("copied stale result")
	}
	awaitResult(tt, "62")
	tt.Key(ui.Cmd, ui.KeyZ)
	awaitResult(tt, "57")
	if n.State.Buffer.String() != text {
		t.Fatal("calculation polluted undo history")
	}
	n.AddBlock()
	n.SetLanguage("Math")
	tt.Frame()
	tt.Type("prev + 1")
	tt.Advance(time.Second)
	tt.SetClipboard("no result")
	// Waiting for the worker must not turn the new block's prev into 57.
	for i := 0; i < 15; i++ {
		time.Sleep(20 * time.Millisecond)
		tt.Advance(100 * time.Millisecond)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if tt.Clipboard() != "no result" {
		t.Fatal("prev leaked between blocks")
	}
	tt.Key(ui.Cmd, ui.KeyN)
	tt.Type("plain text: 12 x 3")
	tt.Advance(time.Second)
	if _, ok := tt.Find("Copy result"); ok {
		t.Fatal("Math controls leaked to plain text")
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	restored.Switch(0)
	app := desktop.New(restored, store)
	defer app.Close()
	rt := ui.NewTester(app.View, 760, 660)
	first := restored.Current()
	first.State.Select(len(text), len(text))
	rt.Frame()
	awaitResult(rt, "57")
	if first.State.Buffer.String() != n.State.Buffer.String() {
		t.Fatal("restart changed source")
	}
	// Switching language invalidates result copying, undo recomputes it.
	first.SetLanguage("Plain Text")
	rt.Frame()
	if _, ok := rt.Find("Copy result"); ok {
		t.Fatal("old Math control retained")
	}
	first.State.Run(editor.Undo)
	rt.Frame()
	awaitResult(rt, "57")
}

func TestMathLanguageLineVariablesCompletionAndResultClick(t *testing.T) {
	store := notebook.Store{Dir: t.TempDir()}
	a := desktop.New(notebook.NewBook(), store)
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 740)
	tt.Key(ui.Cmd, ui.KeyL)
	tt.Type("math")
	tt.Key(0, ui.KeyEnter)
	n := a.Book.Current()
	if block, _, _ := n.CurrentBlock(); block.Language != "Math" {
		t.Fatal("Cmd+L did not choose language")
	}
	tt.Type("1 月消费 1,000 + 2,000\n2 月消费 1000 + 3000\n\n合计 x1 + x2 = ")
	bind := func(line int, name string) {
		t.Helper()
		n.State.GoTo(editor.Pos{Line: line})
		tt.Frame()
		caret, ok := tt.TextCaret()
		area, found := tt.Find("Single-buffer block editor")
		if !ok || !found {
			t.Fatal("no editor caret")
		}
		// Keep a selection spanning the target: a gutter context action
		// must still address the clicked row, not the selection's head.
		b, _, _ := n.CurrentBlock()
		n.State.Select(b.Body, n.State.Buffer.Len())
		tt.Frame()
		tt.PressButton(1, area.X+12, caret.Y+caret.H/2)
		tt.ReleaseButton(1, area.X+12, caret.Y+caret.H/2)
		if err := tt.ChooseMenuItem("Set line as variable " + name); err != nil {
			t.Fatal(err, tt.Menu())
		}
	}
	bind(1, "x1")
	bind(2, "x2")
	if !strings.Contains(n.State.Buffer.String(), "x1 = 1 月消费") || !strings.Contains(n.State.Buffer.String(), "x2 = 2 月消费") {
		t.Fatal(n.State.Buffer.String())
	}
	n.State.Select(n.State.Buffer.Len(), n.State.Buffer.Len())
	tt.Frame()
	deadline := time.After(time.Minute) // Hang guard only; wait for observable completion.
	for !tt.HasText("Tab to insert result") {
		select {
		case <-deadline:
			t.Fatal("completion never arrived", tt.Texts())
		default:
		}
		tt.Advance(200 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	before := n.State.Buffer.String()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText("Tab to insert result") || n.State.Buffer.String() != before {
		t.Fatal("escape did not dismiss without editing")
	}
	// Moving away and back can request the suggestion again.
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyTab)
	if got := n.State.Buffer.String(); got != before+"7000" {
		t.Fatalf("Tab completion: %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if n.State.Buffer.String() != before {
		t.Fatal("completion was not a single undoable insertion")
	}
	// The grouped decimal hint copies its precise raw value, without
	// moving the source insertion point.
	tt.Type("\n13473.05 + 845.96")
	for {
		tt.Advance(200 * time.Millisecond)
		tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
		if tt.Clipboard() == "14319.01" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("decimal result never arrived")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	caret, _ := tt.TextCaret()
	sels := n.State.Selections()
	tt.SetClipboard("not copied")
	tt.Press(caret.X+45, caret.Y+caret.H/2)
	tt.Release(caret.X+45, caret.Y+caret.H/2)
	if tt.Clipboard() != "14319.01" || !reflect.DeepEqual(n.State.Selections(), sels) {
		t.Fatalf("result click: %q, selections %+v", tt.Clipboard(), n.State.Selections())
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil || restored.Current().State.Buffer.String() != n.State.Buffer.String() {
		t.Fatal("line variables lost on restore", err)
	}
	name, assigned := restored.Current().MathLineVariable(1)
	if name != "x1" || !assigned {
		t.Fatal("restored variable binding missing")
	}
	// Prefix removal/undo is an ordinary edit, not detached metadata.
	restored.Current().ToggleMathLineVariable(1)
	if strings.Contains(restored.Current().State.Buffer.String(), "x1 = 1 月消费") {
		t.Fatal("variable not removed")
	}
	restored.Current().State.Run(editor.Undo)
	if restored.Current().State.Buffer.String() != n.State.Buffer.String() {
		t.Fatal("variable removal undo lost content")
	}
}

func TestMathResultsAlignAcrossChineseTabsAndResize(t *testing.T) {
	text := notebook.Header("Math") + "13473.05 + 845.96\n茶点 120 + 30\n\t1000 + 25\n\n" + notebook.Header("Math") + "1000 + 2000 + 3000 + 4000\n1234 + 5678 = "
	n, err := notebook.NewNote("Aligned results", text)
	if err != nil {
		t.Fatal(err)
	}
	b := notebook.NewBook()
	b.Open[0] = n
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 740)
	n.State.Select(len(text), len(text))
	tt.Frame()
	deadline := time.Now().Add(30 * time.Second)
	for !tt.HasText("Tab to insert result") {
		if time.Now().After(deadline) {
			t.Fatal("missing completion", tt.Texts())
		}
		tt.Advance(200 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	if !tt.HasText("6,912") {
		t.Fatal("completion label is not grouped", tt.Texts())
	}
	if tt.HasText("π") || tt.HasText("≡") {
		t.Fatal("Math completion must show only the result and Tab hint", tt.Texts())
	}
	tt.Key(0, ui.KeyTab)
	if n.State.Buffer.String() != text+"6912" {
		t.Fatal("display grouping leaked into inserted result", n.State.Buffer.String())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if n.State.Buffer.String() != text {
		t.Fatal("completion undo changed source")
	}
	for !tt.HasText("Tab to insert result") {
		if time.Now().After(deadline) {
			t.Fatal("missing results after undo")
		}
		tt.Advance(200 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	tt.Key(0, ui.KeyEscape)
	// Locate the actual shaped hint through its public pointer hit targets.
	// Subtracting the independently expected label width gives its number column.
	column := func(source, result string) float32 {
		t.Helper()
		end := strings.Index(text, source) + len(source)
		n.State.Select(end, end)
		tt.Frame()
		caret, ok := tt.TextCaret()
		area, found := tt.Find("Single-buffer block editor")
		if !ok || !found {
			t.Fatal("editor caret missing")
		}
		last := float32(-1)
		for x := caret.X; x < area.X+area.W; x += .25 {
			if _, at := n.State.HitTest(x-area.X, caret.Y+caret.H/2-area.Y); at == end {
				last = x
			}
		}
		if last < 0 {
			t.Fatal("hint is not clickable", source)
		}
		sels := n.State.Selections()
		tt.SetClipboard("not copied")
		tt.Press(last-1, caret.Y+caret.H/2)
		tt.Release(last-1, caret.Y+caret.H/2)
		if tt.Clipboard() != strings.ReplaceAll(result, ",", "") || !reflect.DeepEqual(n.State.Selections(), sels) {
			t.Fatal("grouped hint click changed copy or caret", source, tt.Clipboard())
		}
		w, _ := ui.ShapeText(result, n.State.Font, 0).Size()
		return last - w
	}
	for _, width := range []int{1080, 760, 1280} {
		tt.SetSize(width, 740)
		tt.Frame()
		first := column("13473.05 + 845.96", "14,319.01")
		for _, row := range []struct{ source, result string }{{"茶点 120 + 30", "150"}, {"\t1000 + 25", "1,025"}} {
			got := column(row.source, row.result)
			if got-first > .5 || first-got > .5 {
				t.Fatalf("width %d: %q column %v != %v", width, row.source, got, first)
			}
		}
		if second := column("1000 + 2000 + 3000 + 4000", "10,000"); second < first+20 {
			t.Fatal("different blocks must align independently", first, second)
		}
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Editor")
	click(t, tt, "Increase Font size")
	click(t, tt, "Increase Font size")
	click(t, tt, "Save settings")
	first := column("13473.05 + 845.96", "14,319.01")
	if got := column("茶点 120 + 30", "150"); got-first > .5 || first-got > .5 {
		t.Fatal("font change left stale result spacing", got, first)
	}
}

func TestMathErrorHoverPreservesSourceAndClearsAfterRepair(t *testing.T) {
	b := notebook.NewBook()
	n := b.Current()
	n.SetLanguage("Math")
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 660)
	tt.Type("房贷\n招行 13,473.05\n2 + * 3\n23 /\n12 + 3")
	awaitCopy := func(want string) {
		t.Helper()
		tt.SetClipboard("waiting")
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			tt.Advance(200 * time.Millisecond)
			tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
			if tt.Clipboard() == want {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatal("missing result", want, tt.Clipboard())
	}
	awaitCopy("15")
	text := n.State.Buffer.String()
	start := strings.Index(text, "2 + * 3")
	n.State.Select(start+2, start+2)
	tt.Frame()
	caret, ok := tt.TextCaret()
	if !ok {
		t.Fatal("no source caret")
	}
	selection := n.State.Selection()
	tt.Move(caret.X, caret.Y+caret.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Check expression\nValue expected (char 5)") || n.State.Selection() != selection || n.State.Buffer.String() != text {
		t.Fatal("diagnostic hover missing or mutated source/selection", tt.Texts())
	}
	tt.SetClipboard("unchanged")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if tt.Clipboard() != "unchanged" {
		t.Fatal("error copied as a result")
	}
	n.State.Select(start, start+len("2 + * 3"))
	tt.Type("2 + 3")
	awaitCopy("5")
	if tt.HasText("Check expression\nValue expected (char 5)") {
		t.Fatal("stale diagnostic after repair")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if n.State.Buffer.String() != text {
		t.Fatal("hover or computation polluted undo")
	}
}

func TestSelectCurrentBlockAndEntireNote(t *testing.T) {
	text := notebook.Header("Go") + "alpha\nbeta\n\n" + notebook.Header("Python") + "中文😀\nlast\n" + notebook.Header("Math")
	n, err := notebook.NewNote("Selections", text)
	if err != nil {
		t.Fatal(err)
	}
	b := notebook.NewBook()
	b.Open[0] = n
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	tt := ui.NewTester(a.View, 760, 660)
	n.State.Select(strings.Index(text, "中文"), strings.Index(text, "中文"))
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Command("copy")
	if tt.Clipboard() != "中文😀\nlast" {
		t.Fatalf("current block copy = %q", tt.Clipboard())
	}
	// Repeating Cmd+A stays within the same block; it does not escalate.
	tt.Key(ui.Cmd, ui.KeyA)
	if n.State.SelectedText() != "中文😀\nlast" {
		t.Fatal("repeated select-all escaped current body")
	}
	tt.Type("replacement")
	if n.State.Buffer.String() != strings.Replace(text, "中文😀\nlast", "replacement", 1) {
		t.Fatal("replacing selected block changed another block or its boundary")
	}
	tt.Command("undo")
	if n.State.Buffer.String() != text {
		t.Fatal("block replacement undo lost content")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	tt.Command("copy")
	if tt.Clipboard() != text {
		t.Fatal("whole-note shortcut did not copy all original blocks")
	}
	tt.Type("blocked")
	if n.State.Buffer.String() != text {
		t.Fatal("whole-note selection bypassed boundary protection")
	}
	// Native Edit > Select All follows the same block semantics, including
	// an empty final block; its body is a valid empty selection.
	n.State.Select(len(text), len(text))
	tt.Command("selectAll")
	if n.State.Selection() != (editor.Selection{Anchor: len(text), Head: len(text)}) {
		t.Fatal("empty block selected structural text")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("replacement")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Command("copy")
	if tt.Clipboard() != "replacement" {
		t.Fatal("block shortcut hijacked the search field")
	}
}
