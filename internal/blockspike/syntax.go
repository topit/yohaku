package blockspike

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

// A small line lexer for languages MyGo does not ship. Only comments and
// multiline strings carry state; there is no parser VM or whole-note copy.
type quote struct {
	open, close string
	escape      byte
	multiline   bool
}

type syntaxRule struct {
	pattern *regexp.Regexp
	kind    editor.TokenKind
}

type syntaxLexer struct {
	comments []string
	blocks   [][2]string
	quotes   []quote
	keywords map[string]editor.TokenKind
	extra    string
	foldCase bool
	rules    []syntaxRule
}

func syntaxRules(pairs ...any) []syntaxRule {
	var rules []syntaxRule
	for i := 0; i < len(pairs); i += 2 {
		rules = append(rules, syntaxRule{regexp.MustCompile("^(?:" + pairs[i].(string) + ")"), pairs[i+1].(editor.TokenKind)})
	}
	return rules
}

func keywordSet(keywords, constants string) map[string]editor.TokenKind {
	m := map[string]editor.TokenKind{}
	for _, word := range strings.Fields(keywords) {
		m[word] = editor.TokenKeyword
	}
	for _, word := range strings.Fields(constants) {
		m[word] = editor.TokenConstant
	}
	return m
}

var syntaxNumber = regexp.MustCompile(`^(?:0[xX][0-9a-fA-F_]+|0[bB][01_]+|(?:[0-9][0-9_]*(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?)`)

func appendToken(dst []editor.Token, end int, kind editor.TokenKind) []editor.Token {
	if len(dst) > 0 {
		last := &dst[len(dst)-1]
		if end <= last.End {
			return dst
		}
		if last.Kind == kind {
			last.End = end
			return dst
		}
	}
	return append(dst, editor.Token{End: end, Kind: kind})
}

func (l *syntaxLexer) Tokenize(line string, state uint64, dst []editor.Token) ([]editor.Token, uint64) {
	for i := 0; i < len(line); {
		if state > 0 && state <= uint64(len(l.blocks)) {
			close := l.blocks[state-1][1]
			end := strings.Index(line[i:], close)
			if end < 0 {
				return appendToken(dst, len(line), editor.TokenComment), state
			}
			i += end + len(close)
			dst, state = appendToken(dst, i, editor.TokenComment), 0
			continue
		}
		if state > uint64(len(l.blocks)) {
			q := l.quotes[state-uint64(len(l.blocks))-1]
			start := i
			for i < len(line) {
				if q.escape != 0 && line[i] == q.escape {
					i++
					if i < len(line) {
						_, size := utf8.DecodeRuneInString(line[i:])
						i += size
					}
				} else if strings.HasPrefix(line[i:], q.close) {
					i += len(q.close)
					state = 0
					break
				} else {
					i++
				}
			}
			if i > start {
				dst = appendToken(dst, i, editor.TokenString)
			}
			if !q.multiline {
				state = 0
			}
			continue
		}
		if line[i] == ' ' || line[i] == '\t' {
			for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
				i++
			}
			dst = appendToken(dst, i, editor.TokenText)
			continue
		}
		matched := false
		for k, block := range l.blocks {
			if strings.HasPrefix(line[i:], block[0]) {
				i += len(block[0])
				dst, state = appendToken(dst, i, editor.TokenComment), uint64(k+1)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		for _, comment := range l.comments {
			if strings.HasPrefix(line[i:], comment) {
				return appendToken(dst, len(line), editor.TokenComment), 0
			}
		}
		for _, rule := range l.rules {
			if at := rule.pattern.FindStringIndex(line[i:]); at != nil {
				i += at[1]
				dst = appendToken(dst, i, rule.kind)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		for k, q := range l.quotes {
			if strings.HasPrefix(line[i:], q.open) {
				i += len(q.open)
				dst, state = appendToken(dst, i, editor.TokenString), uint64(len(l.blocks)+k+1)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if at := syntaxNumber.FindStringIndex(line[i:]); at != nil {
			i += at[1]
			dst = appendToken(dst, i, editor.TokenNumber)
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		kind := editor.TokenText
		end := i + size
		if unicode.IsLetter(r) || r == '_' || strings.ContainsRune(l.extra, r) {
			for end < len(line) {
				c, width := utf8.DecodeRuneInString(line[end:])
				// Hyphenated names are valid, but diagram arrows are not
				// part of the preceding node name (A-->B or A->B).
				if c == '-' && end+1 < len(line) && strings.ContainsRune("-.=>", rune(line[end+1])) {
					break
				}
				if !unicode.IsLetter(c) && !unicode.IsDigit(c) && !unicode.IsMark(c) && c != '_' && !strings.ContainsRune(l.extra, c) {
					break
				}
				end += width
			}
			word := line[i:end]
			if l.foldCase {
				word = strings.ToLower(word)
			}
			if k, ok := l.keywords[word]; ok {
				kind = k
			} else if strings.HasPrefix(strings.TrimLeft(line[end:], " \t"), "(") {
				kind = editor.TokenFunction
			} else if unicode.IsUpper(r) {
				kind = editor.TokenType
			}
		} else if strings.ContainsRune("+-*/%=<>!&|^~?:×÷", r) {
			kind = editor.TokenOperator
		} else if strings.ContainsRune("(){}[],;.", r) {
			kind = editor.TokenPunctuation
		}
		dst, i = appendToken(dst, end, kind), end
	}
	if state > uint64(len(l.blocks)) && !l.quotes[state-uint64(len(l.blocks))-1].multiline {
		state = 0
	}
	return dst, state
}
