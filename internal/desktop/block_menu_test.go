package desktop_test

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"reflect"
	"strings"
	"testing"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func TestBlockMenuTargetsClickedBlockAndRejectsStaleConfirmation(t *testing.T) {
	text := notebook.Header("Go") + "first\n" + notebook.Header("Plain Text") + "second中文"
	n, _ := notebook.NewNote("Menu", text)
	b := notebook.NewBook()
	b.Open[0] = n
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	area, _ := tt.Find("Single-buffer block editor")
	// The second body's line is below its folded divider. Keep a selection
	// across both blocks: right-click inside it must not retarget to its head.
	n.State.Select(n.Blocks()[0].Body, len(text))
	tt.Frame()
	sels := n.State.Selections()
	open := func() { tt.RightClickAt(area.X+100, area.Y+46) }
	open()
	if err := tt.ChooseMenuItem("Copy block text"); err != nil {
		t.Fatal(err, tt.Menu())
	}
	tt.Frame()
	if tt.Clipboard() != "second中文" || !reflect.DeepEqual(sels, n.State.Selections()) {
		t.Fatal("copy changed selection or copied wrong block", tt.Clipboard())
	}
	open()
	if err := tt.ChooseMenuItem("Move block up"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !strings.HasPrefix(n.State.Buffer.String(), notebook.Header("Plain Text")+"second中文") {
		t.Fatal("moved wrong block")
	}
	n.State.Run(editor.Undo)
	tt.Frame()
	if n.State.Buffer.String() != text {
		t.Fatal("menu move undo")
	}
	open()
	if err := tt.ChooseMenuItem("Delete current block…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	n.State.Buffer.TryEdit(editor.Edit{Start: len(text), End: len(text), Text: "!"})
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if n.State.Buffer.String() != text+"!" || n.Message != "Block changed; open the menu again." {
		t.Fatal("stale confirmation edited document", n.Message)
	}
}
