package desktop_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func TestSidebarRelocatesSameNotesAndPreservesHistory(t *testing.T) {
	dir := t.TempDir()
	b := notebook.NewBook()
	b.Rename("First")
	a := desktop.New(b, notebook.Store{Dir: dir})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("original中文")
	n := b.Current()
	st := n.State
	buf := st.Buffer
	n.State.Select(len(notebook.Header("Plain Text")), len(n.State.Buffer.String()))
	tt.Frame()
	sels := n.State.Selections()
	for i := 0; i < 6; i++ {
		click(t, tt, "Toggle sidebar")
		tt.Frame()
	}
	if b.Current() != n || n.State != st || st.Buffer != buf || !reflect.DeepEqual(sels, st.Selections()) {
		t.Fatal("layout replaced document or selection")
	}
	click(t, tt, "Toggle sidebar")
	tt.Frame()
	if _, ok := tt.Find("Notes sidebar"); !ok {
		t.Fatal("missing sidebar")
	}
	for i := 0; i < 30; i++ {
		b.New()
		b.Rename(fmt.Sprintf("Long title · 第 %02d 份笔记 — 余白", i))
	}
	tt.Frame()
	r, ok := tt.Find(b.Current().Title)
	if !ok || r.Y < 44 || r.Y+r.H > 712 {
		t.Fatal("active note not revealed", r)
	}
	if err := tt.RightClick(b.Current().Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Close other tabs"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(b.Open) != 1 || len(b.Closed) != 30 {
		t.Fatal("sidebar context lifecycle differs from tabs")
	}
	b.Reopen()
	tt.Frame()
	b.Switch(1)
	tt.Frame()
	// Restore the original note from the archive without recreating it.
	for len(b.Closed) > 0 {
		b.Reopen()
	}
	for i, x := range b.Open {
		if x == n {
			b.Switch(i)
		}
	}
	tt.Frame()
	st.Run(editor.Undo)
	if strings.HasSuffix(buf.String(), "original中文") {
		t.Fatal("layout lost original undo")
	}
	st.Run(editor.Redo)
	tt.SetSize(760, 480)
	tt.Frame()
	if _, ok := tt.Find("Notes sidebar"); ok {
		t.Fatal("narrow editor squeezed by sidebar")
	}
	if r, ok := tt.Find("Editor surface"); !ok || r.W < 730 {
		t.Fatal("narrow canvas lost width", r)
	}
	tt.SetSize(1080, 720)
	tt.Frame()
	if _, ok := tt.Find("Notes sidebar"); !ok {
		t.Fatal("width fallback erased preference")
	}
	a2 := desktop.New(notebook.NewBook(), notebook.Store{Dir: dir})
	defer a2.Close()
	tt2 := ui.NewTester(a2.View, 1080, 720)
	if _, ok := tt2.Find("Notes sidebar"); !ok {
		t.Fatal("sidebar preference lost on restart")
	}
}

func TestSidebarNewButtonsFollowTabsAndRemainReachable(t *testing.T) {
	b := notebook.NewBook()
	b.Rename("Original")
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("keep original 中文")
	original := b.Current()
	click(t, tt, "Toggle sidebar")
	tt.Frame()
	checkBelow := func() {
		t.Helper()
		last, ok := tt.Find(b.Open[len(b.Open)-1].Title)
		add, found := tt.Find("New note")
		image, richFound := tt.Find("New image & text note")
		if !ok || !found || !richFound || add.Y < last.Y+last.H || image.Y != add.Y || add.X+add.W > image.X || image.X+image.W > 216 || add.Y+add.H > 712 {
			t.Fatalf("new-note row is not below the last tab within the sidebar: last=%+v add=%+v image=%+v", last, add, image)
		}
	}
	checkBelow()
	first, _ := tt.Find("New note")
	click(t, tt, "New note")
	tt.Frame()
	if len(b.Open) != 2 || b.Current().Rich != nil {
		t.Fatal("plus did not create a block note")
	}
	b.Rename("New blocks")
	tt.Frame()
	tt.Type("new 中文")
	if !strings.HasSuffix(b.Current().State.Buffer.String(), "new 中文") {
		t.Fatal("new block note did not receive typing focus")
	}
	checkBelow()
	click(t, tt, "New image & text note")
	tt.Frame()
	if len(b.Open) != 3 || b.Current().Rich == nil {
		t.Fatal("image action did not create a rich note")
	}
	b.Rename("New images")
	tt.Frame()
	tt.Type("图文草稿")
	if !strings.Contains(b.Current().Rich.Text().HTML(), "图文草稿") {
		t.Fatal("new rich note did not receive typing focus")
	}
	checkBelow()
	click(t, tt, "Close New images")
	click(t, tt, "Close New blocks")
	tt.Frame()
	afterClose, _ := tt.Find("New note")
	if len(b.Open) != 1 || afterClose.Y != first.Y || b.Current() != original || !strings.HasSuffix(original.State.Buffer.String(), "keep original 中文") {
		t.Fatal("closing tabs did not move buttons back without changing original")
	}
	for i := 0; i < 30; i++ {
		b.New()
		b.Rename(fmt.Sprintf("Overflow %02d", i))
	}
	tt.Frame()
	checkBelow()
	b.Switch(0)
	tt.Frame()
	tt.Scroll(100, 150, 0, 2000)
	tt.Frame()
	checkBelow()
	click(t, tt, "New note")
	tt.Frame()
	if len(b.Open) != 32 || b.Current().Rich != nil {
		t.Fatal("scrolled new-note button was not clickable")
	}
	tt.SetSize(760, 480)
	tt.Frame()
	for _, label := range []string{"New note", "New image & text note"} {
		if r, ok := tt.Find(label); !ok || r.Y+r.H > 44 {
			t.Fatalf("narrow window did not restore titlebar action %s", label)
		}
	}
	click(t, tt, "New image & text note")
	tt.Frame()
	if len(b.Open) != 33 || b.Current().Rich == nil {
		t.Fatal("narrow-window rich-note action changed")
	}
}
