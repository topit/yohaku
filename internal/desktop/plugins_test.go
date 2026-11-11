package desktop_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func waitPlugin(t *testing.T, tt *ui.Tester, n *notebook.Note) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for n.Message == "Running plugin…" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		tt.Frame()
	}
	if n.Message == "Running plugin…" {
		t.Fatal("plugin did not finish")
	}
}

func TestPluginTrustTransformUndoAndDisable(t *testing.T) {
	dir := t.TempDir()
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: dir})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("Hello 中文  \nworld\t")
	n := b.Current()
	original := n.State.Buffer.String()
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Add example")
	click(t, tt, "Save settings")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("tidy-text")
	if !tt.HasText("No matching notes or commands") {
		t.Fatal("untrusted plugin registered executable commands")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Trust & enable tidy-text")
	click(t, tt, "Save settings")
	tt.Frame()
	runCommand(t, tt, "Tidy current block")
	waitPlugin(t, tt, n)
	if n.State.Buffer.String() != notebook.Header("Plain Text")+"Hello 中文\nworld" {
		t.Fatal(n.State.Buffer.String(), n.Message)
	}
	n.State.Run(editor.Undo)
	tt.Frame()
	if n.State.Buffer.String() != original {
		t.Fatal("plugin edit not one undo")
	}
	n.State.Select(len(notebook.Header("Plain Text")), len(notebook.Header("Plain Text"))+5)
	tt.Frame()
	runCommand(t, tt, "Uppercase selection")
	waitPlugin(t, tt, n)
	if !strings.Contains(n.State.Buffer.String(), "HELLO 中文  ") {
		t.Fatal("selection transform edited other text")
	}
	n.State.Run(editor.Undo)
	tt.Frame()
	if n.State.Buffer.String() != original {
		t.Fatal("selection plugin undo")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Disable plugin tidy-text")
	click(t, tt, "Cancel")
	tt.Frame()
	runCommand(t, tt, "Tidy current block")
	waitPlugin(t, tt, n)
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Disable plugin tidy-text")
	click(t, tt, "Save settings")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("tidy-text")
	if !tt.HasText("No matching notes or commands") {
		t.Fatal("disabled command remains registered")
	}
	tt.Key(0, ui.KeyEscape)
	data, _ := os.ReadFile(filepath.Join(dir, "preferences.json"))
	var prefs struct {
		Plugins []struct {
			Path    string
			Enabled bool
		}
	}
	json.Unmarshal(data, &prefs)
	if len(prefs.Plugins) != 1 || prefs.Plugins[0].Enabled {
		t.Fatal("disabled preference not durable")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Remove plugin tidy-text")
	click(t, tt, "Save settings")
	tt.Frame()
	if _, err := os.Stat(filepath.Join(prefs.Plugins[0].Path, "main.ts")); err != nil {
		t.Fatal("uninstall deleted source")
	}
}

func TestPluginRejectsStaleAndBoundaryEdits(t *testing.T) {
	dir := t.TempDir()
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: dir})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("one")
	n := b.Current()
	n.AddBlock()
	tt.Frame()
	tt.Type("two")
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Add example")
	click(t, tt, "Trust & enable tidy-text")
	click(t, tt, "Save settings")
	tt.Frame()
	data, _ := os.ReadFile(filepath.Join(dir, "preferences.json"))
	var prefs struct{ Plugins []struct{ Path string } }
	json.Unmarshal(data, &prefs)
	source := filepath.Join(prefs.Plugins[0].Path, "main.ts")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(source, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`export function upper(input: {text: string}): string { return input.text.toUpperCase(); }`)
	n.State.Select(n.Blocks()[0].Body, n.State.Buffer.Len())
	tt.Frame()
	sels := n.State.Selections()
	before := n.State.Buffer.String()
	runCommand(t, tt, "Uppercase selection")
	waitPlugin(t, tt, n)
	if n.State.Buffer.String() != before || !reflect.DeepEqual(sels, n.State.Selections()) || n.Message != notebook.Rejected {
		t.Fatal("plugin bypassed boundary policy or changed rejected selection", n.Message)
	}
	write(`export function tidy(input: {text: string}): string { const end=Date.now()+400;while(Date.now()<end){};return input.text.toUpperCase(); }`)
	n.State.Select(n.Blocks()[1].Body, n.Blocks()[1].Body)
	tt.Frame()
	runCommand(t, tt, "Tidy current block")
	tt.Type("new")
	want := n.State.Buffer.String()
	waitPlugin(t, tt, n)
	if n.State.Buffer.String() != want || n.Message != "Note or selection changed; plugin result was not applied." {
		t.Fatal("stale plugin overwrote new typing", n.Message)
	}
	write(`export function tidy(): string { while(true) {} }`)
	runCommand(t, tt, "Tidy current block")
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Disable plugin tidy-text")
	click(t, tt, "Save settings")
	tt.Frame()
	waitPlugin(t, tt, n)
	// A slow settings interaction may outlast the command's one-second
	// deadline. Disabling cancels a pending command, not an already failed one.
	cancelled := n.Message == "Plugin command cancelled."
	timedOut := strings.HasPrefix(n.Message, "Plugin failed: ") && strings.Contains(n.Message, "context deadline exceeded")
	if n.State.Buffer.String() != want || (!cancelled && !timedOut) {
		t.Fatal("disabled plugin edited the note or had an unexpected outcome", n.Message)
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("tidy-text")
	if !tt.HasText("No matching notes or commands") {
		t.Fatal("disabled plugin remains executable")
	}
}

func TestPluginSettingsFailureAndRestart(t *testing.T) {
	dir := t.TempDir()
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: dir})
	t.Cleanup(func() { a.Close() })
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Plugins")
	click(t, tt, "Add example")
	click(t, tt, "Trust & enable tidy-text")
	path := filepath.Join(dir, "preferences.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	_, draftKept := tt.Find("Disable plugin tidy-text")
	if !tt.HasText("Settings not saved:") || !draftKept {
		t.Fatal("failed save lost trusted draft")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	tt.Frame()
	a.Close()
	a = desktop.New(b, notebook.Store{Dir: dir})
	tt = ui.NewTester(a.View, 1080, 720)
	tt.Type("hello 中文")
	runCommand(t, tt, "Tidy current block")
	waitPlugin(t, tt, b.Current())
	if b.Current().Message != "Plugin made no changes." {
		t.Fatal("enabled registration did not survive restart", b.Current().Message)
	}
}
