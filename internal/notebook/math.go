package notebook

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/mathnote"
)

// MathLineVariable returns an existing assignment or the next unused xN in
// this block. Lines outside Math and blank/comment lines cannot be bound.
func (n *Note) MathLineVariable(line int) (name string, assigned bool) {
	if n.Rich != nil {
		return "", false
	}
	s := n.State.Buffer.Snapshot()
	if line < 0 || line >= s.LineCount() {
		return "", false
	}
	at := s.LineStart(line)
	blocks, _ := Blocks(s.String())
	for _, b := range blocks {
		if b.Language != "Math" || at < b.Body || at >= b.End {
			continue
		}
		text := strings.TrimSpace(s.Line(line))
		if text == "" || strings.HasPrefix(text, "#") {
			return "", false
		}
		if name := mathnote.LineVariable(text); name != "" {
			return name, true
		}
		used := map[string]bool{}
		for _, row := range strings.Split(s.Slice(b.Body, b.End), "\n") {
			used[mathnote.LineVariable(row)] = true
		}
		for i := 1; ; i++ {
			name := fmt.Sprintf("x%d", i)
			if !used[name] {
				return name, false
			}
		}
	}
	return "", false
}

// ToggleMathLineVariable is an ordinary, undoable text edit. No offset-based
// metadata can drift away from a line on insertion, deletion or restart.
func (n *Note) ToggleMathLineVariable(line int) {
	name, assigned := n.MathLineVariable(line)
	if name == "" {
		return
	}
	s := n.State.Buffer.Snapshot()
	start := s.LineStart(line)
	if assigned {
		text := s.Line(line)
		end := strings.IndexByte(text, '=') + 1
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
		n.State.Select(start, start+end)
		n.State.Run(editor.Backspace)
	} else {
		n.State.Select(start, start)
		n.State.Insert(name + " = ")
	}
}
