package blockspike

import (
	"regexp"

	"github.com/egoist/mygo/ui/editor"
)

// JSX/TSX retain the native JS/TS lexer for code and overlay native markup
// tokens for tags. This is lexical highlighting, not a JSX syntax validator.
// Intern opaque upstream states rather than assuming their bit allocation.
type jsxState struct{ code, markup uint64 }
type jsxTokenizer struct {
	code, markup editor.Tokenizer
	states       []jsxState
	ids          map[jsxState]uint64
}

func newJSXTokenizer(language string) *jsxTokenizer {
	return &jsxTokenizer{code: editor.LanguageNamed(language).Tokenizer, markup: editor.LanguageNamed("HTML").Tokenizer,
		states: []jsxState{{}}, ids: map[jsxState]uint64{{}: 0}}
}

var jsxTag = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9:._-]*(?:[\s/>]|$)|<>|</>`)

func (t *jsxTokenizer) Tokenize(line string, state uint64, dst []editor.Token) ([]editor.Token, uint64) {
	s := t.states[state]
	code, next := t.code.Tokenize(line, s.code, nil)
	s.code = next
	start := -1
	if s.markup != 0 {
		start = 0
	} else {
		for _, match := range jsxTag.FindAllStringIndex(line, -1) {
			kind := kindAt(code, match[0])
			if kind != editor.TokenComment && kind != editor.TokenString && kind != editor.TokenEscape && kind != editor.TokenRegexp {
				start = match[0]
				break
			}
		}
	}
	var markup []editor.Token
	if start >= 0 {
		markup, s.markup = t.markup.Tokenize(line[start:], s.markup, nil)
		for i := range markup {
			markup[i].End += start
		}
		if start > 0 {
			markup = append([]editor.Token{{End: start, Kind: editor.TokenText}}, markup...)
		}
	}
	for i, j, pos := 0, 0, 0; pos < len(line); {
		for i < len(code) && code[i].End <= pos {
			i++
		}
		for j < len(markup) && markup[j].End <= pos {
			j++
		}
		end, kind := len(line), editor.TokenText
		if i < len(code) {
			end, kind = code[i].End, code[i].Kind
		}
		if j < len(markup) {
			end = min(end, markup[j].End)
			mk := markup[j].Kind
			if mk != editor.TokenText && kind != editor.TokenComment && kind != editor.TokenString && kind != editor.TokenEscape && kind != editor.TokenRegexp {
				// Keep code expressions such as count={value + 1} colored as
				// code, rather than treating every byte as an HTML attribute.
				if mk != editor.TokenAttribute || kind != editor.TokenNumber && kind != editor.TokenKeyword && kind != editor.TokenOperator && kind != editor.TokenPunctuation {
					kind = mk
				}
			}
		}
		dst, pos = appendToken(dst, end, kind), end
	}
	id, ok := t.ids[s]
	if !ok {
		id = uint64(len(t.states))
		t.ids[s], t.states = id, append(t.states, s)
	}
	return dst, id
}

func kindAt(tokens []editor.Token, pos int) editor.TokenKind {
	for _, token := range tokens {
		if token.End > pos {
			return token.Kind
		}
	}
	return editor.TokenText
}
