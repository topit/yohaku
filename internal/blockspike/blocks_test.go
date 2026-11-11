package blockspike

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func hasKind(tokens []editor.Token, kind editor.TokenKind) bool {
	for _, token := range tokens {
		if token.Kind == kind {
			return true
		}
	}
	return false
}

func TestBlockHighlightResetsLexicalStateAndFollowsEdit(t *testing.T) {
	text := "⟦yohaku:Go⟧\n/* open\n⟦yohaku:Python⟧\ndef shared():\n    return 42\n"
	b := editor.NewBuffer(text)
	h := editor.NewTokenHighlighter(NewTokenizer(), b.Snapshot())
	h.Highlight(b.LineCount()-1, time.Second)
	check := func(line int, want editor.TokenKind, unwanted editor.TokenKind) {
		t.Helper()
		tokens, ready := h.Tokens(line, b.Line(line), nil)
		if !ready || !hasKind(tokens, want) || hasKind(tokens, unwanted) {
			t.Fatalf("line %d %q: tokens=%+v ready=%v, want %v not %v", line, b.Line(line), tokens, ready, want, unwanted)
		}
	}
	check(1, editor.TokenComment, editor.TokenKeyword)
	check(3, editor.TokenKeyword, editor.TokenComment)
	// An edit inside the first block shifts the marker down. The new Python
	// block must still reset the open Go comment, through Highlighter.Edit.
	old := b.Snapshot()
	at := strings.Index(text, "/* open") + len("/* open")
	b.Edit(editor.Edit{Start: at, End: at, Text: "\nstill comment"})
	h.Edit(old, b.Snapshot(), []editor.Change{{OldStart: at, OldEnd: at, NewStart: at, NewEnd: at + len("\nstill comment")}})
	h.Highlight(b.LineCount()-1, time.Second)
	check(2, editor.TokenComment, editor.TokenKeyword)
	check(4, editor.TokenKeyword, editor.TokenComment)
	// Changing a marker changes its body's language, not just its own color.
	old = b.Snapshot()
	at = strings.Index(b.String(), "Python")
	b.Edit(editor.Edit{Start: at, End: at + 6, Text: "Plain Text"})
	h.Edit(old, b.Snapshot(), []editor.Change{{OldStart: at, OldEnd: at + 6, NewStart: at, NewEnd: at + 10}})
	h.Highlight(b.LineCount()-1, time.Second)
	check(4, editor.TokenText, editor.TokenKeyword)
}

func TestExpandedLanguagesHighlightTokensAndResetAtBoundaries(t *testing.T) {
	for _, name := range Languages {
		if name == "Plain Text" {
			continue
		}
		if language := languageNamed(name); language == nil || language.Tokenizer == nil {
			t.Fatalf("%s is selectable but has no highlighter", name)
		}
	}
	for _, sample := range []struct {
		language, text, needle string
		kind                   editor.TokenKind
	}{
		{"C", "#include <stdio.h>", "#include", editor.TokenMeta},
		{"C++", "class Space {};", "class", editor.TokenKeyword},
		{"C#", "namespace Space {}", "namespace", editor.TokenKeyword},
		{"Java", "public class Space {}", "public", editor.TokenKeyword},
		{"Kotlin", "fun thought() = 42", "fun", editor.TokenKeyword},
		{"Swift", "let thought = 42", "let", editor.TokenKeyword},
		{"Dart", "final thought = 42;", "final", editor.TokenKeyword},
		{"Scala", "def thought = 42", "def", editor.TokenKeyword},
		{"Ruby", "def thought; end", "def", editor.TokenKeyword},
		{"Lua", "local thought = 42", "local", editor.TokenKeyword},
		{"PHP", "<?php $thought = 42;", "$thought", editor.TokenVariable},
		{"XML", `<note title="余白">`, "note", editor.TokenTag},
		{"TOML", `title = "余白"`, `"余白"`, editor.TokenString},
		{"Diff", "+a little room", "+a little room", editor.TokenInserted},
		{"R", "thought <- function(x) x + 1", "function", editor.TokenKeyword},
		{"Dockerfile", "FROM alpine:latest", "FROM", editor.TokenKeyword},
		{"Makefile", "# build thoughts", "# build thoughts", editor.TokenComment},
		{"Protobuf", "message Thought {}", "message", editor.TokenKeyword},
		{"GraphQL", "query Thought { title }", "query", editor.TokenKeyword},
		{"Zig", "const thought = 42;", "const", editor.TokenKeyword},
		{"JSX", `const view = <Button title="余白" count={2 + 1} />;`, "Button", editor.TokenTag},
		{"TSX", `const view: Props = <Button count={2 + 1} />;`, "Button", editor.TokenTag},
		{"Vue", `<template><p class="thought">余白</p></template>`, "template", editor.TokenTag},
		{"Clojure", `(defn thought [x] (+ x 1))`, "defn", editor.TokenKeyword},
		{"Elixir", "def thought, do: :ok", ":ok", editor.TokenConstant},
		{"Erlang", "-module(thought).", "-module", editor.TokenMeta},
		{"Groovy", "def thought = '余白'", "def", editor.TokenKeyword},
		{"PowerShell", "FUNCTION Get-Thought { $value = 42 }", "FUNCTION", editor.TokenKeyword},
		{"Mermaid", "flowchart LR\n  A --> B", "flowchart", editor.TokenKeyword},
		{"Mermaid", "  A-->B", "-->", editor.TokenOperator},
		{"Mermaid", "stateDiagram-v2", "stateDiagram-v2", editor.TokenKeyword},
		{"Lezer", "@top Thought { Word+ }", "@top", editor.TokenMeta},
		{"Math", "sqrt(81) + 12 × 3", "81", editor.TokenNumber},
	} {
		t.Run(sample.language, func(t *testing.T) {
			text := "⟦yohaku:Groovy⟧\n/* unclosed\n⟦yohaku:" + sample.language + "⟧\n" + sample.text + "\n⟦yohaku:Plain Text⟧\nconst flowchart defn 42\n"
			buffer := editor.NewBuffer(text)
			h := editor.NewTokenHighlighter(NewTokenizer(), buffer.Snapshot())
			if !h.Highlight(buffer.LineCount()-1, time.Second) {
				t.Fatal("highlighting did not finish")
			}
			tokens, ready := h.Tokens(3, buffer.Line(3), nil)
			start := strings.Index(buffer.Line(3), sample.needle)
			if !ready || start < 0 {
				t.Fatal("invalid sample or unfinished highlighting")
			}
			for at := start; at < start+len(sample.needle); at++ {
				if got := kindAt(tokens, at); got != sample.kind {
					t.Fatalf("%q has token kind %v, want %v; tokens=%+v", sample.needle, got, sample.kind, tokens)
				}
			}
			plain := buffer.LineCount() - 2
			tokens, ready = h.Tokens(plain, buffer.Line(plain), nil)
			if !ready || len(tokens) != 1 || tokens[0].Kind != editor.TokenText {
				t.Fatal("highlight state leaked into Plain Text", tokens)
			}
		})
	}
}

func TestMixedSyntaxStringsCommentsAndMultilineStates(t *testing.T) {
	for _, language := range []string{"JSX", "TSX"} {
		lexer := languageNamed(language).Tokenizer
		state := uint64(0)
		for _, line := range []string{`const text = "<NotATag>"; // <Comment>`, `/* <AlsoNotATag>`, `still comment */ const view = <Button`, `  title="余白" count={2 + 1} />;`} {
			tokens, next := lexer.Tokenize(line, state, nil)
			if strings.Contains(line, "NotATag") && hasKind(tokens, editor.TokenTag) {
				t.Fatal("markup overrode a string/comment", language, tokens)
			}
			if strings.Contains(line, "<Button") && kindAt(tokens, strings.Index(line, "Button")) != editor.TokenTag {
				t.Fatal("tag after closing multiline comment was not highlighted", language, tokens)
			}
			if strings.Contains(line, "count=") && (kindAt(tokens, strings.Index(line, "2")) != editor.TokenNumber || kindAt(tokens, strings.Index(line, "title")) != editor.TokenAttribute) {
				t.Fatal("multiline attributes/code expression lost", language, tokens)
			}
			state = next
		}
	}
	for _, sample := range []struct{ language, open, close string }{
		{"Groovy", "'''余白", "more text''' + 42"},
		{"Elixir", `"""余白`, `more text""" + 42`},
		{"PowerShell", `@"余白`, `more text"@ + 42`},
	} {
		lexer := languageNamed(sample.language).Tokenizer
		_, state := lexer.Tokenize(sample.open, 0, nil)
		tokens, state := lexer.Tokenize("still a string // # 中文", state, nil)
		if state == 0 || len(tokens) != 1 || tokens[0].Kind != editor.TokenString {
			t.Fatal("multiline string not preserved", sample.language, tokens)
		}
		tokens, _ = lexer.Tokenize(sample.close, state, nil)
		if kindAt(tokens, strings.LastIndex(sample.close, "42")) != editor.TokenNumber {
			t.Fatal("closing a string did not resume code", sample.language, tokens)
		}
	}
}

func harness(st *editor.State) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		Decorate(st)
		editor.View(c, st).Fill().AutoFocus()
	}, 900, 600)
}

func TestSingleBufferCrossBlockSelectionCopyUndoAndSearch(t *testing.T) {
	st := NewState(Sample)
	tt := harness(st)
	a := strings.Index(Sample, "func shared")
	z := strings.Index(Sample, "def shared") + len("def shared")
	st.Select(a, z)
	if got := st.SelectedText(); got != Sample[a:z] {
		t.Fatalf("cross-block selection = %q", got)
	}
	st.Run(editor.Copy)
	if got := tt.Clipboard(); got != Sample[a:z] {
		t.Fatalf("cross-block clipboard = %q", got)
	}
	st.Insert("replacement 跨块")
	want := Sample[:a] + "replacement 跨块" + Sample[z:]
	if got := st.Buffer.String(); got != want {
		t.Fatalf("cross-block paste = %q", got)
	}
	st.Run(editor.Undo)
	if st.Buffer.String() != Sample || st.SelectedText() != Sample[a:z] {
		t.Fatal("undo did not restore text and cross-block selection")
	}
	st.Run(editor.Redo)
	if st.Buffer.String() != want {
		t.Fatal("redo did not restore replacement")
	}
	st.Run(editor.Undo)
	st.Select(0, 0)
	st.Find("shared")
	if n, _ := st.FindMatches(); n != 3 {
		t.Fatalf("shared matches across prose, Go and Python = %d, want 3", n)
	}
	// The public search also accepts a query spanning a real block boundary.
	st.Find("block\n\n⟦yohaku:Python⟧\ndef")
	if n, _ := st.FindMatches(); n != 1 {
		t.Fatalf("cross-boundary multiline matches = %d, want 1", n)
	}
}

// These are characterization tests of a missing capability, not assertions
// that destroying delimiters is desirable. An eventual framework policy
// must reverse these observations without resetting text or undo history.
func TestOnInputCannotProtectAgainstIMEComposition(t *testing.T) {
	st := NewState(Sample)
	calls := 0
	st.OnInput = func(ui.InputEvent) bool { calls++; return true }
	tt := harness(st)
	a := strings.Index(Sample, "⟦yohaku:Go⟧")
	st.Select(a, a+len("⟦yohaku:Go⟧"))
	tt.Frame()
	tt.Type("blocked")
	if st.Buffer.String() != Sample || calls != 1 {
		t.Fatalf("ordinary input was not blocked: calls=%d", calls)
	}
	tt.Compose("中文", 2)
	tt.Frame()
	tt.Type("中文") // commit, not an ordinary InputText callback
	tt.Frame()
	want := strings.Replace(Sample, "⟦yohaku:Go⟧", "中文", 1)
	if got := st.Buffer.String(); got != want || calls != 1 {
		t.Fatalf("IME bypass observation: text=%q calls=%d", got, calls)
	}
	st.Run(editor.Undo)
	if st.Buffer.String() != Sample {
		t.Fatal("composition and commit did not undo together")
	}
	t.Log("OnInput blocked ordinary typing but received no composition/commit; selected delimiter was replaced. This is simulated text service input, NOT a native Chinese IME test.")
}

func TestOnInputCannotProtectReplaceAllOrBufferEdit(t *testing.T) {
	st := NewState(Sample)
	calls := 0
	st.OnInput = func(ui.InputEvent) bool { calls++; return true }
	var replaceAll func()
	st.RenderFindBar = func(c *ui.Context, bar ui.Element, f *editor.FindBar) {
		*f.Replacement() = "REMOVED"
		replaceAll = f.ReplaceAll
		editor.DefaultFindBar(c, bar, f)
	}
	tt := harness(st)
	st.Find("⟦yohaku:Go⟧")
	tt.Frame()
	if replaceAll == nil {
		t.Fatal("find bar did not render")
	}
	replaceAll()
	if st.Buffer.String() != strings.Replace(Sample, "⟦yohaku:Go⟧", "REMOVED", 1) || calls != 0 {
		t.Fatalf("replace-all bypass: calls=%d text=%q", calls, st.Buffer.String())
	}
	st.Run(editor.Undo)
	if st.Buffer.String() != Sample {
		t.Fatal("replace-all undo lost text")
	}
	a := strings.Index(Sample, "⟦yohaku:Go⟧")
	st.Buffer.Edit(editor.Edit{Start: a, End: a + len("⟦yohaku:Go⟧"), Text: "DELETED"})
	if st.Buffer.String() != strings.Replace(Sample, "⟦yohaku:Go⟧", "DELETED", 1) || calls != 0 {
		t.Fatal("Buffer.Edit observation changed")
	}
	t.Log("ReplaceAll and Buffer.Edit replaced structural bytes with no OnInput callback.")
}

func TestBlockLineNumbersRestartAndFollowEdits(t *testing.T) {
	st := NewState("⟦yohaku:Go⟧\nalpha\nbeta\n⟦yohaku:Python⟧\n中文😀\n\n⟦yohaku:Math⟧\n")
	var presentation Presentation
	colors := [2]ui.Color{ui.Hex("#ffffff"), ui.Hex("#eeeeee")}
	check := func(want []int) {
		t.Helper()
		presentation.Decorate(st, colors)
		if st.LineNumbers != editor.LineNumbersOn || st.Buffer.LineCount() != len(want) {
			t.Fatal("line numbers disabled or unexpected text")
		}
		for line, n := range want {
			if got := st.LineNumber(line); got != n {
				t.Fatalf("line %d number = %d, want %d", line, got, n)
			}
		}
	}
	check([]int{0, 1, 2, 0, 1, 2, 0, 1})
	st.Select(len("⟦yohaku:Go⟧\nalpha"), len("⟦yohaku:Go⟧\nalpha"))
	st.Insert("\nextra")
	check([]int{0, 1, 2, 3, 0, 1, 2, 0, 1})
	st.Run(editor.Undo)
	check([]int{0, 1, 2, 0, 1, 2, 0, 1})
	colors = [2]ui.Color{ui.Hex("#222222"), ui.Hex("#333333")}
	if got := presentation.Decorate(st, colors); got != colors[0] || st.LineBackgrounds[1].Color != colors[1] {
		t.Fatal("palette change did not refresh backgrounds")
	}
	st = NewState("⟦yohaku:Plain Text⟧\nother\n⟦yohaku:Go⟧\nlast")
	check([]int{0, 1, 0, 1})
}

func BenchmarkBlockPresentation(b *testing.B) {
	st := NewState(strings.Repeat("⟦yohaku:Go⟧\nfirst\nsecond 中文\nthird\n\n", 1000))
	var presentation Presentation
	colors := [2]ui.Color{ui.Hex("#ffffff"), ui.Hex("#eeeeee")}
	presentation.Decorate(st, colors)
	b.ReportAllocs()
	for b.Loop() {
		presentation.Decorate(st, colors)
	}
}
