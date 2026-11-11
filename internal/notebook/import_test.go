package notebook_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/notebook"
)

func TestHeynoteLibraryImportPreservesBodiesAndExistingTabs(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.txt":               "\ufeff{\"formatVersion\":\"2.0.0\",\"name\":\"旅行 😀\",\"tags\":[\"keep source\"]}\r\n∞∞∞text;created=2026-11-09T00:00:00Z\r\n中文 😀\r\n\r\n\r\n∞∞∞python-a\r\nprint('ok')\r\n",
		"nested/buffer.txt":   "\n∞∞∞golang-a\npackage main\n∞∞∞math\nradius = 7\nradius^2 * PI\n∞∞∞unknown\nclass Draft {}",
		".images/ignored.txt": "not a note", "ignored.bin": "not a note",
	}
	for path, data := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "nested/buffer.txt"), filepath.Join(dir, "skip-link.txt")); err != nil {
		t.Log("symlink control unavailable:", err)
	}
	b := notebook.NewBook()
	old := b.Current()
	old.State.Insert("existing draft")
	old.State.Run(editor.Undo)
	count, warnings, err := b.ImportHeynote([]string{dir, filepath.Join(dir, "a.txt")})
	if err != nil || count != 2 || len(b.Open) != 3 || b.Active != 1 {
		t.Fatalf("import: count=%d open=%d active=%d err=%v", count, len(b.Open), b.Active, err)
	}
	if b.Open[0] != old {
		t.Fatal("existing buffer replaced")
	}
	old.State.Run(editor.Redo)
	if old.State.Buffer.String() != notebook.Header("Plain Text")+"existing draft" {
		t.Fatal("import damaged existing redo")
	}
	if b.Open[1].Title != "旅行 😀" || b.Open[1].State.Buffer.String() != "⟦yohaku:Plain Text⟧\n中文 😀\n\n\n⟦yohaku:Python⟧\nprint('ok')\n" {
		t.Fatalf("modern note differs: %q %q", b.Open[1].Title, b.Open[1].State.Buffer.String())
	}
	if b.Open[2].Title != "buffer" || b.Open[2].State.Buffer.String() != "⟦yohaku:Go⟧\npackage main\n⟦yohaku:Math⟧\nradius = 7\nradius^2 * PI\n⟦yohaku:Plain Text⟧\nclass Draft {}" {
		t.Fatalf("legacy note differs: %q", b.Open[2].State.Buffer.String())
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unknown") {
		t.Fatalf("missing compatibility warnings: %v", warnings)
	}
	store := notebook.Store{Dir: t.TempDir()}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil || !reflect.DeepEqual(b.Snapshot(), restored.Snapshot()) {
		t.Fatal("import did not survive restart", err)
	}
	for path, expected := range files {
		actual, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(actual) != expected {
			t.Fatalf("source changed: %s", path)
		}
	}
}

func TestHeynoteImportAllLanguageModes(t *testing.T) {
	// Canonical tokens from Heynote's 37-mode catalog, not derived from our
	// importer map: a missing/incorrect alias must fail this round trip.
	modes := [][2]string{
		{"text", "Plain Text"}, {"math", "Math"}, {"json", "JSON"}, {"python", "Python"},
		{"html", "HTML"}, {"sql", "SQL"}, {"markdown", "Markdown"}, {"java", "Java"},
		{"lezer", "Lezer"}, {"php", "PHP"}, {"css", "CSS"}, {"xml", "XML"},
		{"cpp", "C++"}, {"rust", "Rust"}, {"csharp", "C#"}, {"ruby", "Ruby"},
		{"shell", "Shell"}, {"yaml", "YAML"}, {"toml", "TOML"}, {"golang", "Go"},
		{"clojure", "Clojure"}, {"elixir", "Elixir"}, {"erlang", "Erlang"}, {"javascript", "JavaScript"},
		{"jsx", "JSX"}, {"typescript", "TypeScript"}, {"tsx", "TSX"}, {"swift", "Swift"},
		{"kotlin", "Kotlin"}, {"lua", "Lua"}, {"groovy", "Groovy"}, {"diff", "Diff"},
		{"powershell", "PowerShell"}, {"vue", "Vue"}, {"dart", "Dart"}, {"scala", "Scala"}, {"mermaid", "Mermaid"},
	}
	var source []string
	for i, mode := range modes {
		source = append(source, fmt.Sprintf("∞∞∞%s\nkept %d 中文 😀", mode[0], i))
	}
	path := filepath.Join(t.TempDir(), "languages.txt")
	if err := os.WriteFile(path, []byte(strings.Join(source, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	b := notebook.NewBook()
	if count, warnings, err := b.ImportHeynote([]string{path}); err != nil || count != 1 || len(warnings) != 0 {
		t.Fatal("recognized modes were downgraded", count, warnings, err)
	}
	store := notebook.Store{Dir: t.TempDir()}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	text := restored.Current().State.Buffer.String()
	blocks, err := notebook.Blocks(text)
	if err != nil || len(blocks) != len(modes) {
		t.Fatal("languages did not survive restart", len(blocks), err)
	}
	for i, block := range blocks {
		if block.Language != modes[i][1] || text[block.Body:block.End] != fmt.Sprintf("kept %d 中文 😀", i) {
			t.Fatalf("%s changed language/body: %+v", modes[i][0], block)
		}
	}
}

func TestHeynoteImportRejectsWholeBatch(t *testing.T) {
	for name, invalid := range map[string]string{
		"metadata":         "not JSON\n∞∞∞text\nbody",
		"future":           "{\"formatVersion\":\"3.0.0\"}\n∞∞∞text\nbody",
		"bad version":      "{\"formatVersion\":\"2.x.0\"}\n∞∞∞text\nbody",
		"not Heynote":      "an ordinary text file",
		"UTF-8":            "\n∞∞∞text\n\xff",
		"binary":           "\n∞∞∞text\n\x00",
		"collision":        "\n∞∞∞text\nmention ⟦yohaku:Plain Text⟧",
		"incomplete":       "\n∞∞∞text",
		"adjacent headers": "\n∞∞∞text\n∞∞∞math\n42",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			good, bad := filepath.Join(dir, "good.txt"), filepath.Join(dir, "bad.txt")
			if err := os.WriteFile(good, []byte("\n∞∞∞text\nkeep me"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bad, []byte(invalid), 0600); err != nil {
				t.Fatal(err)
			}
			b := notebook.NewBook()
			b.Current().State.Insert("unsaved existing")
			before, revision := b.Snapshot(), b.Revision
			count, _, err := b.ImportHeynote([]string{good, bad})
			if err == nil || count != 0 || b.Revision != revision || !reflect.DeepEqual(before, b.Snapshot()) {
				t.Fatal("failed batch partially imported or changed live state", err)
			}
		})
	}
}

func TestHeynoteImportEmptyBlocksAndCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("∞∞∞text\n\n∞∞∞math\n"), 0600); err != nil {
		t.Fatal(err)
	}
	b := notebook.NewBook()
	count, _, err := b.ImportHeynote([]string{path})
	if err != nil || count != 1 || b.Current().State.Buffer.String() != "⟦yohaku:Plain Text⟧\n\n⟦yohaku:Math⟧\n" {
		t.Fatal("empty blocks lost", err)
	}
	before := b.Snapshot()
	if count, _, err := b.ImportHeynote(nil); err != nil || count != 0 || !reflect.DeepEqual(before, b.Snapshot()) {
		t.Fatal("cancel changed session")
	}
	if count, _, err := b.ImportHeynote([]string{filepath.Join(dir, "missing.txt")}); err == nil || count != 0 || !reflect.DeepEqual(before, b.Snapshot()) {
		t.Fatal("unreadable source changed session")
	}
}
