package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/blockspike"
)

func TestTabSwitchPreservesIndependentEditorState(t *testing.T) {
	original := blockspike.Sample + strings.Repeat("room for another thought\n", 60)
	first, second := blockspike.NewState(original), blockspike.NewState("second tab")
	a := &app{states: []*editor.State{first, second}}
	tt := ui.NewTester(a.view, 1050, 700)
	first.Select(len(original), len(original))
	first.Insert("first edit")
	first.Select(100, 160)
	tt.Frame()
	first.SetScrollY(160)
	tt.Frame()
	sel, scroll := first.Selection(), first.ScrollY()
	if scroll <= 0 {
		t.Fatal("test did not exercise a scrolled editor")
	}
	if err := tt.Click("Another tab"); err != nil {
		t.Fatal(err)
	}
	if a.active != 1 {
		t.Fatal("second tab did not activate")
	}
	second.Select(0, 0)
	second.Insert("other ")
	if err := tt.Click("Scratchpad"); err != nil {
		t.Fatal(err)
	}
	if a.active != 0 || first.Selection() != sel || first.ScrollY() != scroll {
		t.Fatalf("state after returning: active=%d selection=%+v scroll=%v; want %+v %v", a.active, first.Selection(), first.ScrollY(), sel, scroll)
	}
	if first.Buffer.String() != original+"first edit" || second.Buffer.String() != "other second tab" {
		t.Fatal("tab text was lost or mixed")
	}
	first.Run(editor.Undo)
	if first.Buffer.String() != original || second.Buffer.String() != "other second tab" {
		t.Fatal("first tab undo touched the second tab or lost its history")
	}
	second.Run(editor.Undo)
	if second.Buffer.String() != "second tab" {
		t.Fatal("second tab lost its independent history")
	}
}
