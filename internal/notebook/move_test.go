package notebook_test

import (
	"github.com/egoist/mygo/ui/editor"
	"testing"
	"yohaku/internal/notebook"
)

func TestMoveBlockIsOneUndoableSwap(t *testing.T) {
	first, second, third := notebook.Header("Go")+"中文\nhello", notebook.Header("Math")+"12 + 3", notebook.Header("Plain Text")
	original := first + "\n" + second + "\n" + third
	for _, dir := range []int{-1, 1} {
		n := note(t, original)
		at := n.Blocks()[1].Body + 2
		n.State.Select(at, at)
		n.MoveBlock(dir)
		want := second + "\n" + first + "\n" + third
		if dir > 0 {
			want = first + "\n" + third + "\n" + second
		}
		if n.State.Buffer.String() != want {
			t.Fatal(n.State.Buffer.String())
		}
		b, _, _ := n.CurrentBlock()
		if b.Language != "Math" || n.State.Cursor() != b.Body+2 {
			t.Fatal("lost block/caret identity")
		}
		n.State.Run(editor.Undo)
		if n.State.Buffer.String() != original {
			t.Fatal("move not one undo")
		}
		n.State.Run(editor.Redo)
		if n.State.Buffer.String() != want {
			t.Fatal("redo lost body or language")
		}
	}
}
