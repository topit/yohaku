// Package notebook owns the single-buffer document model and tab lifecycle.
package notebook

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
	"yohaku/internal/blockspike"
)

const Prefix = "⟦yohaku:"
const Rejected = "Edit blocked: block boundaries are protected. Select and copy across blocks, or use block commands."

var Languages = blockspike.Languages

func Header(language string) string { return Prefix + language + "⟧\n" }

// Block ranges count UTF-8 bytes. End excludes the newline preceding the next
// header: that newline and the entire header form a protected boundary.
type Block struct {
	Start, Body, End int
	Language         string
}

func Blocks(text string) ([]Block, error) {
	var out []Block
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.Contains(line, Prefix) {
			name, ok := blockspike.Marker(strings.TrimSuffix(line, "\n"))
			if !ok || !strings.HasSuffix(line, "\n") || !slices.Contains(Languages, name) {
				return nil, fmt.Errorf("invalid block header at byte %d", offset)
			}
			if len(out) > 0 {
				if out[len(out)-1].Body > offset-1 {
					return nil, errors.New("blocks must have a separating newline")
				}
				out[len(out)-1].End = offset - 1
			}
			out = append(out, Block{Start: offset, Body: offset + len(line), End: len(text), Language: name})
		}
		offset += len(line)
	}
	if len(out) == 0 || out[0].Start != 0 {
		return nil, errors.New("document must start with a block header")
	}
	return out, nil
}

type authorization struct {
	version uint64
	edits   []editor.Edit
}

// Note is main-thread owned. Saving snapshots does not mutate live buffers.
type Note struct {
	ID, Title     string
	State         *editor.State
	Message       string
	PendingScroll *float64
	Presentation  blockspike.Presentation
	blocks        []Block
	blockVersion  uint64
	authorized    *authorization
	Rich          *richtext.Document
	RichScroll    float64
	stored        *storedNote
	images        *imageCache
}

func newID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

func NewNote(title, text string) (*Note, error) {
	blocks, err := Blocks(text)
	if err != nil {
		return nil, err
	}
	n := &Note{ID: newID(), Title: title, State: blockspike.NewState(text), images: &imageCache{}}
	n.State.SoftWrap = true
	n.State.Placeholder = "Leave room for a thought…"
	n.State.Buffer.SetEditPolicy(n.policy)
	n.State.OnInput = n.HandleInput
	n.State.Select(blocks[0].Body, blocks[0].Body)
	return n, nil
}

// Handle native editing commands and editor keys here so find/rename fields
// and rich notes keep their normal input behavior.
func (n *Note) HandleInput(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods == ui.Alt {
		n.AddBlockBefore()
		return true
	}
	backspace := ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyBackspace && ev.Mods == 0 ||
		ev.Kind == ui.InputCommand && ev.Text == "delete"
	if backspace {
		sels := n.State.Selections()
		b, index, _ := n.CurrentBlock()
		if len(sels) == 1 && sels[0].Anchor == sels[0].Head && b.Body == b.End {
			if index > 0 {
				n.RemoveBlock()
			}
			return true
		}
	}
	all := ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyA && ev.Mods == ui.Cmd|ui.Shift
	block := ev.Kind == ui.InputCommand && ev.Text == "selectAll" ||
		ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyA && ev.Mods == ui.Cmd
	if all {
		n.State.Run(editor.SelectAll)
	} else if block {
		b, _, _ := n.CurrentBlock()
		n.State.Select(b.Body, b.End)
	} else {
		return false
	}
	return true
}

func (n *Note) policy(before editor.Snapshot, edits []editor.Edit) bool {
	if a := n.authorized; a != nil && a.version == before.Version() && slices.Equal(a.edits, edits) {
		return true
	}
	blocks := n.blocksAt(before)
	if len(blocks) == 0 {
		n.Message = Rejected
		return false
	}
	for _, e := range edits {
		if strings.Contains(e.Text, Prefix) {
			n.Message = Rejected
			return false
		}
		for _, b := range blocks {
			start := b.Start
			if start > 0 {
				start--
			}
			if e.Start < b.Body && e.End > start || e.Start == e.End && e.Start >= b.Start && e.Start < b.Body {
				n.Message = Rejected
				return false
			}
		}
	}
	// The reserved prefix must not be assembled across successive edits.
	// Validate the candidate too, not just each insertion in isolation.
	var candidate strings.Builder
	last := 0
	for _, e := range edits {
		candidate.WriteString(before.Slice(last, e.Start))
		candidate.WriteString(e.Text)
		last = e.End
	}
	candidate.WriteString(before.Slice(last, before.Len()))
	if next, err := Blocks(candidate.String()); err != nil || len(next) != len(blocks) {
		n.Message = Rejected
		return false
	}
	return true
}

// Blocks returns the current immutable block ranges. Callers must not modify them.
func (n *Note) Blocks() []Block { return n.blocksAt(n.State.Buffer.Snapshot()) }

func (n *Note) blocksAt(s editor.Snapshot) []Block {
	if n.blocks == nil || n.blockVersion != s.Version() {
		n.blocks, _ = Blocks(s.String())
		n.blockVersion = s.Version()
	}
	return n.blocks
}

func (n *Note) CurrentBlock() (Block, int, int) {
	blocks := n.Blocks()
	i := 0
	for j, b := range blocks {
		if n.State.Cursor() >= b.Start {
			i = j
		}
	}
	return blocks[i], i, len(blocks)
}

func (n *Note) structure(e editor.Edit, caret int) bool {
	n.authorized = &authorization{version: n.State.Buffer.Version(), edits: []editor.Edit{e}}
	defer func() { n.authorized = nil }()
	if !n.State.Buffer.TryEdit(e) {
		return false
	}
	blocks, _ := Blocks(n.State.Buffer.String())
	for _, b := range blocks {
		if caret >= b.Start && caret < b.Body {
			caret = b.Body
			break
		}
	}
	n.State.Select(caret, caret)
	n.Message = ""
	return true
}

func (n *Note) AddBlock() {
	b, _, _ := n.CurrentBlock()
	text := "\n" + Header("Plain Text")
	n.structure(editor.Edit{Start: b.End, End: b.End, Text: text}, b.End+len(text))
}

func (n *Note) AddBlockBefore() {
	b, _, _ := n.CurrentBlock()
	text := Header("Plain Text") + "\n"
	n.structure(editor.Edit{Start: b.Start, End: b.Start, Text: text}, b.Start+len(Header("Plain Text")))
}

func (n *Note) SplitBlock() {
	b, _, _ := n.CurrentBlock()
	at := max(b.Body, min(n.State.Cursor(), b.End))
	text := "\n" + Header(b.Language)
	n.structure(editor.Edit{Start: at, End: at, Text: text}, at+len(text))
}

// MoveBlock swaps adjacent complete blocks in a single undoable transaction.
func (n *Note) MoveBlock(direction int) {
	b, index, count := n.CurrentBlock()
	other := index + direction
	if (direction != -1 && direction != 1) || other < 0 || other >= count {
		return
	}
	blocks := n.Blocks()
	left, right := blocks[min(index, other)], blocks[max(index, other)]
	s := n.State.Buffer.Snapshot()
	text := s.Slice(right.Start, right.End) + "\n" + s.Slice(left.Start, left.End)
	caret := left.Start + n.State.Cursor() - b.Start
	if direction > 0 {
		caret += right.End - right.Start + 1
	}
	n.structure(editor.Edit{Start: left.Start, End: right.End, Text: text}, caret)
}

func (n *Note) RemoveBlock() {
	b, index, count := n.CurrentBlock()
	if count == 1 {
		n.structure(editor.Edit{Start: 0, End: n.State.Buffer.Len(), Text: Header("Plain Text")}, len(Header("Plain Text")))
		return
	}
	start, end := b.Start-1, b.End
	if index == 0 {
		start, end = 0, b.End+1
	}
	n.structure(editor.Edit{Start: start, End: end}, max(0, start))
}

func (n *Note) SetLanguage(language string) {
	if !slices.Contains(Languages, language) {
		return
	}
	b, _, _ := n.CurrentBlock()
	if language == b.Language {
		return
	}
	text := Header(language)
	caret := max(b.Start+len(text), n.State.Cursor()+len(text)-(b.Body-b.Start))
	n.structure(editor.Edit{Start: b.Start, End: b.Body, Text: text}, caret)
}

type Book struct {
	Open, Closed         []*Note
	Active               int
	Revision             uint64
	HeynoteImportHandled bool
	images               imageCache
}

func NewBook() *Book           { b := &Book{}; b.New(); return b }
func (b *Book) Current() *Note { return b.Open[b.Active] }
func (b *Book) New() {
	n, _ := NewNote("Untitled", Header("Plain Text"))
	n.images = &b.images
	b.Open = append(b.Open, n)
	b.Active = len(b.Open) - 1
	b.Revision++
}
func (b *Book) Switch(i int) {
	if i >= 0 && i < len(b.Open) && i != b.Active {
		b.Active = i
		b.Revision++
	}
}
func (b *Book) Rename(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}
	if b.Current().Title != title {
		b.Current().Title = title
		b.Revision++
	}
}
func (b *Book) Close(i int) {
	if i < 0 || i >= len(b.Open) {
		return
	}
	b.Closed = append(b.Closed, b.Open[i])
	b.Open = slices.Delete(b.Open, i, i+1)
	if i < b.Active {
		b.Active--
	}
	b.Active = max(0, min(b.Active, len(b.Open)-1))
	if len(b.Open) == 0 {
		b.New()
	}
	b.Revision++
}
func (b *Book) Reopen() {
	b.ReopenAt(len(b.Closed) - 1)
}

// ReopenAt restores a specific closed note without recreating its editor state.
func (b *Book) ReopenAt(i int) {
	if i < 0 || i >= len(b.Closed) {
		return
	}
	n := b.Closed[i]
	b.Closed = slices.Delete(b.Closed, i, i+1)
	b.Open = append(b.Open, n)
	b.Active = len(b.Open) - 1
	b.Revision++
}
