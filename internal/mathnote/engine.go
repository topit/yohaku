// Package mathnote evaluates bounded, offline Math blocks, independently of
// the native UI. Results are derived data, never edits to the source document.
package mathnote

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MoeForge/moejs"
)

//go:embed vendor/math.js
var library string

//go:embed evaluate.js
var evaluator string

var (
	programOnce sync.Once
	program     *moejs.Script
	programErr  error
)

// Block has the original UTF-8 offset of its body, excluding its delimiter.
type Block struct {
	Start int
	Text  string
}

type Result struct {
	Start, End    int
	Display, Copy string
	Error         string
	Complete      bool // A trailing result '=' can offer a completion.
	Written       bool // The annotation already equals the current result.
}

// Engine is owned by one worker, not shared between goroutines. The JS VM has
// no host bindings. AST restrictions exclude ranges, matrices, accessors,
// strings, scripts and user functions; the timeout is not a memory sandbox.
type Engine struct {
	vm    *moejs.Runtime
	begin moejs.Value
	calc  moejs.Value
}

func NewEngine() (*Engine, error) {
	programOnce.Do(func() { program, programErr = moejs.CompileScript("mathjs-14.8.1.js", library) })
	if programErr != nil {
		return nil, programErr
	}
	// MoeJS uses its default call-depth limit; it has no public equivalent of
	// Goja's configurable 512-frame limit. AST and calculation limits still apply.
	vm := moejs.NewRuntime(moejs.Options{})
	if _, err := vm.RunScript(program); err != nil {
		return nil, err
	}
	adapter, err := moejs.CompileScript("evaluate.js", evaluator)
	if err != nil {
		return nil, err
	}
	if _, err := vm.RunScript(adapter); err != nil {
		return nil, err
	}
	lookup, err := moejs.CompileScript("lookup.js", "[beginBlock, calculate]")
	if err != nil {
		return nil, err
	}
	functions, err := vm.RunScript(lookup)
	if err != nil {
		return nil, err
	}
	begin, err := vm.Get(functions, "0")
	if err != nil {
		return nil, err
	}
	calc, err := vm.Get(functions, "1")
	if err != nil {
		return nil, err
	}
	vm.ReleaseCallData()
	return &Engine{vm: vm, begin: begin, calc: calc}, nil
}

// Evaluate recomputes from each block's beginning, with independent scopes.
// A bad line leaves prev at the last successful value. A cancelled/over-budget
// document returns no partial results; its next evaluation starts fresh.
func (e *Engine) Evaluate(ctx context.Context, blocks []Block) ([]Result, error) {
	bytes, lines := 0, 0
	for _, b := range blocks {
		bytes += len(b.Text)
		lines += strings.Count(b.Text, "\n") + 1
	}
	if bytes > 128*1024 || lines > 2000 {
		return nil, errors.New("Math limit: 128 KiB and 2000 lines per note")
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	// Synchronize the interrupt goroutine before clearing it: a late interrupt
	// from the previous job must never poison the next evaluation.
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			e.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	defer func() { close(done); <-stopped; e.vm.ClearInterrupt(); e.vm.ReleaseCallData() }()
	var out []Result
	for _, b := range blocks {
		if _, err := e.vm.CallFunction(e.begin, moejs.Undefined()); err != nil {
			return nil, fmt.Errorf("Math calculation interrupted: %w", err)
		}
		offset := b.Start
		for _, line := range strings.Split(b.Text, "\n") {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("Math calculation interrupted: %w", err)
			}
			r := Result{Start: offset, End: offset + len(line)}
			offset += len(line) + 1
			if len(line) > 4096 {
				r.Error = "Math limit: 4096 bytes per line"
			} else {
				value, err := e.vm.CallFunction(e.calc, moejs.Undefined(), moejs.String(normalize(ungroupNumbers(line))))
				if err != nil {
					return nil, fmt.Errorf("Math calculation interrupted: %w", err)
				}
				for name, dest := range map[string]*bool{"complete": &r.Complete, "written": &r.Written} {
					v, err := e.vm.Get(value, name)
					if err != nil {
						return nil, err
					}
					*dest = !v.IsUndefined() && v.String() == "true"
				}
				for name, dest := range map[string]*string{"display": &r.Display, "copy": &r.Copy, "error": &r.Error} {
					v, err := e.vm.Get(value, name)
					if err != nil {
						return nil, err
					}
					if !v.IsUndefined() {
						*dest = v.String()
					}
				}
			}
			r.Display = groupDisplay(r.Display)
			if r.Display != "" || r.Error != "" {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// Group only ordinary decimal values (including a leading unit coefficient).
// Copy stays untouched: no float conversion, rounding or exponent rewriting.
func groupDisplay(value string) string {
	number, unit, hasUnit := strings.Cut(value, " ")
	if hasUnit && (strings.HasPrefix(unit, "+") || strings.HasPrefix(unit, "-")) {
		return value // A complex value, not a unit coefficient.
	}
	sign := ""
	if strings.HasPrefix(number, "-") || strings.HasPrefix(number, "+") {
		sign, number = number[:1], number[1:]
	}
	integer, fraction, decimal := strings.Cut(number, ".")
	if len(integer) <= 3 {
		return value
	}
	for _, digits := range []string{integer, fraction} {
		for _, digit := range digits {
			if digit < '0' || digit > '9' {
				return value // Scientific notation and complex values keep their format.
			}
		}
	}
	var out strings.Builder
	out.WriteString(sign)
	for i, digit := range integer {
		if i > 0 && (len(integer)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	if decimal {
		out.WriteByte('.')
		out.WriteString(fraction)
	}
	if hasUnit {
		out.WriteByte(' ')
		out.WriteString(unit)
	}
	return out.String()
}

// normalize recognizes an infix x/X as multiplication, not x in names, base
// literals, quotes or comments. An x at the start of an expression remains a
// variable; 12x3 is the deliberately calculator-like shorthand.
func normalize(s string) string {
	var out strings.Builder
	operand := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '#' {
			out.WriteString(s[i:])
			break
		}
		if r == '\'' || r == '"' {
			start := i
			i += size
			for i < len(s) {
				c := s[i]
				i++
				if c == '\\' && i < len(s) {
					i++
				} else if c == byte(r) {
					break
				}
			}
			out.WriteString(s[start:i])
			operand = true
			continue
		}
		if (r == 'x' || r == 'X') && operand {
			tail := s[i+size:]
			next := strings.TrimLeftFunc(tail, unicode.IsSpace)
			n, _ := utf8.DecodeRuneInString(next)
			// A name starting x is not an operator, except a numeric suffix
			// immediately after an operand (12x3, not a name like tax).
			boundary := tail == "" || !identifier(rune(tail[0])) ||
				tail[0] >= '0' && tail[0] <= '9' && i > 0 && s[i-1] != ' ' && s[i-1] != '\t'
			if boundary && (next == "" || n == '#' || identifier(n) || n == '(' || n == '.' || n == '+' || n == '-') {
				out.WriteByte('*')
				i += size
				operand = false
				continue
			}
		}
		if r >= '0' && r <= '9' || r == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			start := i
			i += size
			if r == '0' && i < len(s) && strings.ContainsRune("xXbBoO", rune(s[i])) {
				i++
				for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] >= 'a' && s[i] <= 'f' || s[i] >= 'A' && s[i] <= 'F') {
					i++
				}
			} else {
				for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
					i++
				}
				if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
					j := i + 1
					if j < len(s) && (s[j] == '+' || s[j] == '-') {
						j++
					}
					if j < len(s) && s[j] >= '0' && s[j] <= '9' {
						i = j + 1
						for i < len(s) && s[i] >= '0' && s[i] <= '9' {
							i++
						}
					}
				}
			}
			out.WriteString(s[start:i])
			operand = true
			continue
		}
		if identifier(r) {
			start := i
			i += size
			for i < len(s) {
				n, sz := utf8.DecodeRuneInString(s[i:])
				if !identifier(n) {
					break
				}
				i += sz
			}
			out.WriteString(s[start:i])
			operand = true
			continue
		}
		if r == '×' {
			out.WriteByte('*')
		} else if r == '÷' {
			out.WriteByte('/')
		} else {
			out.WriteRune(r)
		}
		if !unicode.IsSpace(r) {
			operand = r == ')' || r == ']'
		}
		i += size
	}
	return out.String()
}

func identifier(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}
