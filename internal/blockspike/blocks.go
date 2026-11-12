// Package blockspike provides single-buffer block highlighting and presentation.
// Its diagnostic executable deliberately leaves boundaries editable; the
// notebook package adds transaction policy and the durable document contract.
package blockspike

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const Sample = `⟦yohaku:Markdown⟧
# A little room to think

余白 — one tab, several languages, one continuous selection.
Keep a thought here. Try searching for shared across all three blocks.

⟦yohaku:Go⟧
package main

func shared() string {
    return "less, but better"
}
/* deliberately unclosed: must not leak into the Python block

⟦yohaku:Python⟧
def shared():
    return "留一点空白"
`

// Marker recognizes only complete spike marker lines. Marker collision,
// escaping and immutable identities remain document-model decisions.
func Marker(line string) (string, bool) {
	name, ok := strings.CutPrefix(line, "⟦yohaku:")
	if !ok || !strings.HasSuffix(name, "⟧") {
		return "", false
	}
	return strings.TrimSuffix(name, "⟧"), true
}

type lexicalState struct {
	language *editor.Language
	inner    uint64
}

// Tokenizer uses opaque interned states, not reserved bits in the underlying
// tokenizers' uint64. A marker resets the next block to lexical state zero.
// Each buffer gets a separate tokenizer, as MyGo highlighters are main-thread.
type Tokenizer struct {
	states    []lexicalState
	ids       map[lexicalState]uint64
	languages map[string]*editor.Language
}

func NewTokenizer() *Tokenizer {
	zero := lexicalState{}
	return &Tokenizer{states: []lexicalState{zero}, ids: map[lexicalState]uint64{zero: 0}, languages: map[string]*editor.Language{}}
}

func (t *Tokenizer) intern(s lexicalState) uint64 {
	if id, ok := t.ids[s]; ok {
		return id
	}
	id := uint64(len(t.states))
	t.states = append(t.states, s)
	t.ids[s] = id
	return id
}

func (t *Tokenizer) Tokenize(line string, state uint64, dst []editor.Token) ([]editor.Token, uint64) {
	if name, ok := Marker(line); ok {
		dst = append(dst, editor.Token{End: len(line), Kind: editor.TokenMeta})
		language, found := t.languages[name]
		if !found {
			language = languageNamed(name)
			t.languages[name] = language
		}
		return dst, t.intern(lexicalState{language: language})
	}
	s := t.states[state]
	if s.language == nil || s.language.Tokenizer == nil {
		return append(dst, editor.Token{End: len(line), Kind: editor.TokenText}), state
	}
	dst, s.inner = s.language.Tokenizer.Tokenize(line, s.inner, dst)
	return dst, t.intern(s)
}

func NewState(text string) *editor.State {
	b := editor.NewBuffer(text)
	b.SetLanguage(&editor.Language{Name: "Yohaku blocks (spike)", Tokenizer: NewTokenizer()})
	return &editor.State{
		Buffer: b, Font: ui.Font{Size: 15}, LineHeight: 1.8,
		LineNumbers: editor.LineNumbersOn, NoIndentGuides: true,
		NoFolding: true, NoActiveLine: true, NoAutoClose: true,
		Label: "Single-buffer block editor",
	}
}

// Presentation keeps a state's decoration until its text or palette changes.
// The state owns the derived slices; scrolling and layout do not invalidate them.
type Presentation struct {
	buffer     *editor.Buffer
	version    uint64
	colors     [2]ui.Color
	background ui.Color
}

func (p *Presentation) Decorate(st *editor.State, colors [2]ui.Color) ui.Color {
	version := st.Buffer.Version()
	if p.buffer != st.Buffer || p.version != version || p.colors != colors {
		p.background = Decorate(st, colors)
		p.buffer, p.version, p.colors = st.Buffer, version, colors
	}
	return p.background
}

// Decorate presents structural lines as rules, with paper for prose and an
// alternate background for code and Math. The trailing canvas stays paper.
// No text, selections or history are changed by presentation.
func Decorate(st *editor.State, blockColors ...[2]ui.Color) ui.Color {
	st.LineBackgrounds = st.LineBackgrounds[:0]
	st.TextBackgrounds = st.TextBackgrounds[:0]
	st.DividerLines = st.DividerLines[:0]
	s := st.Buffer.Snapshot()
	numbers := make([]int, s.LineCount())
	number := 0
	colors := [2]ui.Color{ui.Hex("#fffefd"), ui.Hex("#f4f2f7")}
	if len(blockColors) > 0 {
		colors = blockColors[0]
	}
	background := colors[0]
	for i := range s.LineCount() {
		line := s.Line(i)
		if language, ok := Marker(line); ok {
			number = 0
			count := len(st.LineBackgrounds)
			if count > 0 {
				st.LineBackgrounds[count-1].Last = i - 1
			}
			background = colors[0]
			if language != "Plain Text" && language != "Markdown" {
				background = colors[1]
			}
			st.LineBackgrounds = append(st.LineBackgrounds, editor.LineBackground{
				First: i, Last: s.LineCount() - 1, Color: background,
			})
			if !slices.Contains(st.DividerLines, line) {
				st.DividerLines = append(st.DividerLines, line)
			}
		} else {
			number++
			numbers[i] = number
		}
	}
	st.LineNumber = func(line int) int {
		if line >= 0 && line < len(numbers) {
			return numbers[line]
		}
		return 0
	}
	return colors[0]
}
