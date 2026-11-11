package mathnote

import (
	"regexp"
	"strings"
	"unicode"
)

var assignment = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z_0-9]*)\s*=`)
var numberToken = regexp.MustCompile(`^[0-9][0-9,]*(\.[0-9]*)?([eE][+-]?[0-9]*)?`)
var groupedNumber = regexp.MustCompile(`^[0-9]{1,3}(,[0-9]{3})+(\.[0-9]*)?([eE][+-]?[0-9]*)?$`)

// LineVariable recognizes a visible scalar assignment, not a hidden line ID.
// The prefix travels with ordinary edits, undo, copy and session recovery.
func LineVariable(line string) string {
	m := assignment.FindStringSubmatch(line)
	if m == nil || strings.HasPrefix(strings.TrimSpace(line[len(m[0]):]), "=") {
		return ""
	}
	return m[1]
}

// ungroupNumbers accepts complete thousands groups outside function arguments.
// Inside calls commas always separate arguments: max(1,234) still has two.
// Parenthesized arithmetic, e.g. (1,234 + 5), may use grouped numbers.
func ungroupNumbers(s string) string {
	var out strings.Builder
	var calls []bool
	inCall := false
	previous := rune(0)
	for i := 0; i < len(s); {
		c := s[i]
		if c == '#' || c == '\'' || c == '"' {
			out.WriteString(s[i:])
			break
		}
		if c == '(' {
			calls = append(calls, inCall)
			inCall = inCall || unicode.IsLetter(previous) || previous == '_'
		} else if c == ')' && len(calls) > 0 {
			inCall, calls = calls[len(calls)-1], calls[:len(calls)-1]
		}
		identifier := i > 0 && (s[i-1] >= 0x80 || s[i-1] == '_' ||
			s[i-1] >= 'a' && s[i-1] <= 'z' || s[i-1] >= 'A' && s[i-1] <= 'Z' ||
			s[i-1] >= '0' && s[i-1] <= '9')
		if c >= '0' && c <= '9' && !inCall && !identifier {
			value := numberToken.FindString(s[i:])
			if groupedNumber.MatchString(value) {
				out.WriteString(strings.ReplaceAll(value, ",", ""))
			} else {
				out.WriteString(value)
			}
			i += len(value)
			previous = '0'
			continue
		}
		out.WriteByte(c)
		if c != ' ' && c != '\t' {
			previous = rune(c)
		}
		i++
	}
	return out.String()
}
