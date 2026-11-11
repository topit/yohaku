package blockspike

import (
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// Keep persisted names stable. The same catalog drives menus and import;
// aliases are for import/search, not alternative document marker spellings.
var Languages = []string{
	"Plain Text", "Markdown", "Math", "Go", "Python", "JavaScript", "TypeScript", "JSON", "SQL", "Shell", "HTML", "CSS", "Rust", "YAML",
	"C", "C++", "C#", "Java", "Kotlin", "Swift", "Dart", "Scala", "Ruby", "Lua", "PHP", "XML", "TOML", "Diff",
	"JSX", "TSX", "Vue", "Clojure", "Elixir", "Erlang", "Groovy", "PowerShell", "Mermaid", "Lezer",
	"R", "Dockerfile", "Makefile", "Protobuf", "GraphQL", "Zig",
}

var extraLanguages = makeExtraLanguages()

func languageNamed(name string) *editor.Language {
	// Stateful mixed-mode adapters belong to one buffer, not a global cache.
	if name == "JSX" || name == "TSX" {
		base := "JavaScript"
		if name == "TSX" {
			base = "TypeScript"
		}
		return &editor.Language{Name: name, Tokenizer: newJSXTokenizer(base)}
	}
	if language := extraLanguages[name]; language != nil {
		return language
	}
	return editor.LanguageNamed(name)
}

// HeynoteLanguage accepts Heynote's canonical tokens plus usual aliases.
// Unknown languages remain unknown so the importer can report its fallback.
func HeynoteLanguage(token string) string {
	switch token {
	case "text":
		return "Plain Text"
	case "math":
		return "Math"
	}
	for _, name := range Languages {
		if strings.EqualFold(name, token) {
			return name
		}
	}
	if language := editor.LanguageNamed(token); language != nil {
		return language.Name
	}
	return ""
}

func makeExtraLanguages() map[string]*editor.Language {
	out := map[string]*editor.Language{}
	add := func(name string, lexer editor.Tokenizer) { out[name] = &editor.Language{Name: name, Tokenizer: lexer} }
	quoted := []quote{{"\"", "\"", '\\', false}, {"'", "'", '\\', false}}
	triple := append([]quote{{"\"\"\"", "\"\"\"", '\\', true}, {"'''", "'''", '\\', true}}, quoted...)
	add("Clojure", &syntaxLexer{comments: []string{";"}, quotes: []quote{{"#\"", "\"", '\\', true}, {"\"", "\"", '\\', true}}, extra: "-?!*+/<>=.$%&:#", keywords: keywordSet("def defn defn- defmacro defmulti defmethod defonce defprotocol defrecord deftype fn let letfn loop recur if if-let if-not when when-let when-not cond case do doseq for lazy-seq try catch finally throw ns in-ns require import quote var set! new and or not", "true false nil"), rules: syntaxRules(`:[\pL_][\pL\pN_./?!*-]*`, editor.TokenConstant)})
	add("Elixir", &syntaxLexer{comments: []string{"#"}, quotes: triple, extra: "?!", keywords: keywordSet("def defp defmodule defprotocol defimpl defmacro defmacrop defstruct defdelegate do end fn when case cond if unless else rescue catch after try receive with for quote unquote import require alias use and or not in", "true false nil"), rules: syntaxRules(`:[\pL_][\pL\pN_?!]*`, editor.TokenConstant, `@[\pL_][\pL\pN_]*`, editor.TokenMeta)})
	add("Erlang", &syntaxLexer{comments: []string{"%"}, quotes: quoted, extra: "@", keywords: keywordSet("after begin case try cond catch andalso orelse end fun if let of receive when bnot not div rem band and bor bxor bsl bsr or xor maybe else", "true false undefined ok error"), rules: syntaxRules(`-(?:module|export|import|include|define|record|spec|type|callback)\b`, editor.TokenMeta, `'[^'\n]*'`, editor.TokenConstant)})
	add("Groovy", &syntaxLexer{comments: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}, quotes: triple, extra: "$", keywords: keywordSet("as assert break case catch class const continue def default do else enum extends final finally for if implements import in instanceof interface native new package private protected public return static super switch synchronized this throw throws trait transient try void volatile while", "true false null")})
	add("PowerShell", &syntaxLexer{comments: []string{"#"}, blocks: [][2]string{{"<#", "#>"}}, quotes: []quote{{"@\"", "\"@", '`', true}, {"@'", "'@", 0, true}, {"\"", "\"", '`', false}, {"'", "'", 0, false}}, foldCase: true, extra: "-", keywords: keywordSet("begin break catch class continue data define do dynamicparam else elseif end enum exit filter finally for foreach from function if in param process return switch throw trap try until using var while workflow", "true false null"), rules: syntaxRules(`\$(?:\{[^}]*\}|[\pL_?][\pL\pN_:?]*)`, editor.TokenVariable, `-[a-zA-Z][a-zA-Z0-9]*`, editor.TokenOperator)})
	add("Mermaid", &syntaxLexer{comments: []string{"%%"}, quotes: quoted, extra: "-", keywords: keywordSet("graph flowchart subgraph end direction sequenceDiagram participant actor as Note note over left right of loop alt else opt par and rect critical option break autonumber activate deactivate title classDiagram class stateDiagram stateDiagram-v2 state fork join choice erDiagram gantt dateFormat axisFormat excludes section pie showData journey gitGraph commit branch checkout merge cherry-pick mindmap timeline quadrantChart xychart-beta block-beta architecture-beta packet-beta sankey-beta requirementDiagram requirement", "LR RL TB TD BT"), rules: syntaxRules(`(?:<[-.=]+>|[-.=]+>|<[-.=]+|[-.=]{2,}|:::)`, editor.TokenOperator)})
	add("Lezer", &syntaxLexer{comments: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}, quotes: quoted, keywords: keywordSet("std eof", ""), rules: syntaxRules(`@[a-zA-Z][a-zA-Z0-9]*`, editor.TokenMeta, `!?\[[^\]]*\]`, editor.TokenString, `\$\[[^\]]*\]`, editor.TokenString)})
	add("Math", &syntaxLexer{comments: []string{"#", "//"}, keywords: keywordSet("in to and or not mod", "pi PI e i true false Infinity NaN")})
	// MyGo's HTML lexer already resets and dispatches <script>/<style> bodies.
	add("Vue", editor.LanguageNamed("HTML").Tokenizer)
	return out
}
