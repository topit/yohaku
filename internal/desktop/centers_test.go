package desktop_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func runCommand(t *testing.T, tt *ui.Tester, query string) {
	t.Helper()
	tt.Key(ui.Cmd, ui.KeyK)
	if !tt.Focused("Command query") {
		t.Fatal("⌘K did not focus command search", tt.Texts())
	}
	tt.Type(query)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
}

func TestCommandCenterTargetsOriginalEditor(t *testing.T) {
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("left middle right")
	n := b.Current()
	header := len(notebook.Header("Plain Text"))
	n.State.Select(header+5, header+11)
	tt.Frame()
	before := b.Snapshot()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("zzzznonexistent")
	if !tt.HasText("No matching notes or commands") {
		t.Fatal("missing empty state")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !reflect.DeepEqual(before, b.Snapshot()) {
		t.Fatal("cancel changed original document or selection")
	}
	tt.Type("replaced")
	if !strings.HasSuffix(n.State.Buffer.String(), "left replaced right") {
		t.Fatal("cancel did not restore editor focus", n.State.Buffer.String(), tt.Focused("Single-buffer block editor"))
	}
	runCommand(t, tt, "撤销")
	if !strings.HasSuffix(n.State.Buffer.String(), "left middle right") {
		t.Fatal("undo acted on the palette instead of the note")
	}
	runCommand(t, tt, "Change block language")
	tt.Type("Python")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	block, _, _ := n.CurrentBlock()
	if block.Language != "Python" {
		t.Fatal("language subpicker did not execute")
	}
	runCommand(t, tt, "Delete current block")
	if !tt.HasText("Delete this block") {
		t.Fatal("missing explicit deletion confirmation")
	}
	tt.Key(0, ui.KeyEscape)
	if !strings.Contains(n.State.Buffer.String(), "left middle right") {
		t.Fatal("delete executed before confirmation")
	}
	runCommand(t, tt, "Find and replace")
	if !tt.Focused("Find") {
		t.Fatal("find did not focus the source search field", tt.Texts())
	}
	tt.Type("middle")
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("cancelled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !tt.Focused("Find") {
		t.Fatal("palette cancel lost the find field")
	}
}

func TestCommandSwitchAndMathLanguageAlias(t *testing.T) {
	b := notebook.NewBook()
	b.Current().Title = "旅行草稿"
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 480)
	tt.Type("first note")
	first := b.Current()
	runCommand(t, tt, "New block note")
	tt.Type("second note")
	b.Rename("旅行草稿")
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("旅行")
	if !tt.HasText("Switch note · Blocks · 1") || !tt.HasText("Current note · Blocks · 2") {
		t.Fatal("identical titles cannot be distinguished without reading their bodies", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	runCommand(t, tt, "旅行")
	if b.Current() != first {
		t.Fatal("title search did not switch notes")
	}
	runCommand(t, tt, "Change block language")
	tt.Type("计算")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	block, _, _ := first.CurrentBlock()
	if block.Language != "Math" {
		t.Fatal("Chinese Math alias selected another language", block.Language)
	}
	before := first.State.Buffer.String()
	runCommand(t, tt, "Change block language")
	tt.Type("TSX")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	block, _, _ = first.CurrentBlock()
	if block.Language != "TSX" || !strings.HasSuffix(first.State.Buffer.String(), "first note") {
		t.Fatal("expanded language search changed the body or chose another language", block.Language)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if first.State.Buffer.String() != before {
		t.Fatal("language change did not undo as one structure transaction")
	}
	runCommand(t, tt, "Close current note")
	if len(b.Closed) != 1 || !strings.HasSuffix(b.Closed[0].State.Buffer.String(), "first note") {
		t.Fatal("close lost original note")
	}
	runCommand(t, tt, "Reopen last closed")
	if b.Current() != first {
		t.Fatal("reopen did not restore archived note")
	}
}

func TestCommandCompositionAndRichContext(t *testing.T) {
	b := notebook.NewBook()
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 620)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Compose("新建", 2)
	tt.Key(0, ui.KeyEnter)
	if len(b.Open) != 1 || !tt.Focused("Command query") {
		t.Fatal("IME candidate confirmation executed a command")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("New")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if b.Current().Rich == nil {
		t.Fatal("arrow navigation did not select the second matching command")
	}
	tt.Type("bold me")
	b.Current().Rich.SetSelection(0, 4)
	runCommand(t, tt, "Toggle bold")
	if !strings.Contains(b.Current().Rich.Text().HTML(), "<b>bold</b>") {
		t.Fatal("rich command lost original selection", b.Current().Rich.Text().HTML())
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("Split block")
	if !tt.HasText("No matching notes or commands") {
		t.Fatal("block commands leaked into rich context")
	}
}

func TestAllNotesPickerRestoresSpecificClosedNoteAndState(t *testing.T) {
	b := notebook.NewBook()
	b.Rename("Switch note") // A title that also happens to be a translation key.
	first := b.Current()
	first.State.Insert("first")
	first.AddBlock()
	first.SetLanguage("Go")
	first.State.Insert("second")
	first.AddBlock()
	first.State.Insert("third")
	middle := first.Blocks()[1].Body
	first.State.Select(middle, middle)
	b.New()
	b.Rename("Shared")
	older := b.Current()
	older.State.Insert("recover 中文")
	start := len(notebook.Header("Plain Text"))
	older.State.Select(start+1, start+4)
	selection := older.State.Selection()
	b.Close(b.Active)
	b.New()
	b.Rename("Shared")
	newer := b.Current()
	newer.State.Insert("different content")
	b.Close(b.Active)
	b.NewRich()
	b.Rename("图文草稿")
	rich := b.Current()
	b.Close(b.Active)
	b.New()
	b.Rename("Visible")
	b.Current().State.Insert("body-only-needle")
	a := desktop.New(b, notebook.Store{Dir: t.TempDir()})
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 480)
	before := b.Snapshot()
	runCommand(t, tt, "View all notes")
	if !tt.HasText("All notes") || !tt.HasText("Open notes") || !tt.HasText("Closed notes") || !tt.HasText("Closed note · Blocks · 1") || !tt.HasText("Closed note · Blocks · 2") {
		t.Fatal("missing grouped, distinguishable notes", tt.Texts())
	}
	tt.Type("body-only-needle")
	if !tt.HasText("No matching notes") {
		t.Fatal("all-notes search scanned document bodies")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !tt.Focused("Command query") || !tt.HasText("View all notes") || !reflect.DeepEqual(before, b.Snapshot()) {
		t.Fatal("Esc did not return to root without changing notes", tt.Texts())
	}
	click(t, tt, "View all notes")
	tt.Frame()
	click(t, tt, "Back to command center")
	tt.Frame()
	click(t, tt, "View all notes")
	tt.Frame()
	tt.Type("Shared")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if b.Current() != older || len(b.Closed) != 2 || b.Closed[0] != newer || b.Closed[1] != rich || older.State.Selection() != selection {
		t.Fatal("picker restored the wrong note, reordered others or lost selection")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if older.State.Buffer.String() != notebook.Header("Plain Text") {
		t.Fatal("restoring a specific note lost its undo history")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	runCommand(t, tt, "View all notes")
	tt.Type("图文")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if b.Current() != rich || len(b.Closed) != 1 || b.Closed[0] != newer {
		t.Fatal("rich note restore replaced the original or disturbed remaining closed notes")
	}
	if _, ok := tt.Find("Block language"); ok {
		t.Fatal("block language shown for a rich note")
	}
	runCommand(t, tt, "View all notes")
	tt.Type("Switch note")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if b.Current() != b.Open[0] {
		t.Fatal("open-note picker did not switch to the selected original")
	}
	position, found := tt.Find("Block 2 / 3")
	language, ok := tt.Find("Block language")
	if !found || !ok || language.Y < 440 || language.X < 500 || position.Y < 440 || position.X+position.W > language.X || language.X+language.W > 744 {
		t.Fatal("block position and language are not together at bottom-right", position, language)
	}
	click(t, tt, "Block language")
	click(t, tt, "Markdown")
	blocks := first.Blocks()
	if blocks[0].Language != "Plain Text" || blocks[1].Language != "Markdown" || blocks[2].Language != "Plain Text" {
		t.Fatal("footer language picker changed a block other than the cursor's block")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if first.Blocks()[1].Language != "Go" {
		t.Fatal("footer language picker lost editor focus for undo")
	}
	first.State.Select(first.Blocks()[2].Body, first.Blocks()[2].Body)
	tt.Frame()
	if !tt.HasText("Block 3 / 3") || !tt.HasText("Plain Text") {
		t.Fatal("footer did not follow the cursor into the next block")
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "archived") {
			t.Fatal("closed-note count is still displayed", text)
		}
	}
}

func TestThemePreviewApplyFailureAndRestore(t *testing.T) {
	dir := t.TempDir()
	store := notebook.Store{Dir: dir}
	b := notebook.NewBook()
	a := desktop.New(b, store)
	defer a.Close()
	tt := ui.NewTester(a.View, 1080, 720)
	tt.Type("keep my document")
	n := b.Current()
	text, version, original := n.State.Buffer.String(), n.State.Buffer.Version(), n.State.Theme.Text
	a.OpenThemes()
	tt.Frame()
	click(t, tt, "Appearance Dark")
	click(t, tt, "Theme mist")
	tt.Frame()
	if n.State.Theme.Text == original || n.State.Buffer.Version() != version {
		t.Fatal("dark preview did not recolor, or changed source")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if n.State.Theme.Text != original {
		t.Fatal("cancel did not restore colors")
	}
	if _, err := os.Stat(filepath.Join(dir, "preferences.json")); !os.IsNotExist(err) {
		t.Fatal("preview persisted preferences")
	}
	a.OpenThemes()
	tt.Frame()
	click(t, tt, "Appearance Dark")
	click(t, tt, "Theme moss")
	click(t, tt, "Language zh-Hans")
	// Force a preference-only write failure; note saves remain independent.
	if err := os.Mkdir(filepath.Join(dir, "preferences.json"), 0700); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	if !strings.Contains(strings.Join(tt.Texts(), " "), "Settings not saved") || a.SaveError != "" || a.T("Settings") != "Settings" {
		t.Fatal("preference failure hidden or confused with notebook failure")
	}
	if err := a.Flush(); err != nil {
		t.Fatal("preference failure blocked notebook save", err)
	}
	if err := os.Remove(filepath.Join(dir, "preferences.json")); err != nil {
		t.Fatal(err)
	}
	click(t, tt, "Save settings")
	tt.Frame()
	data, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
	var saved map[string]any
	if err != nil || json.Unmarshal(data, &saved) != nil || saved["theme"] != "moss" || saved["mode"] != "dark" {
		t.Fatal("wrong committed preferences", string(data), err)
	}
	if n.State.Buffer.String() != text || n.State.Buffer.Version() != version {
		t.Fatal("theme application edited source")
	}
	n.State.Run(editor.Undo)
	if strings.Contains(n.State.Buffer.String(), "keep my document") {
		t.Fatal("theme reset source undo")
	}
	other := desktop.New(notebook.NewBook(), store)
	defer other.Close()
	ui.NewTester(other.View, 760, 620)
	if other.Book.Current().State.Theme.Text == original {
		t.Fatal("dark theme not restored on restart")
	}
}

func TestThemeFamiliesFollowSystemWithoutEditing(t *testing.T) {
	for _, name := range []string{"yohaku", "paper", "mist", "moss"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"theme":"`+name+`","mode":"system"}`), 0600); err != nil {
				t.Fatal(err)
			}
			a := desktop.New(notebook.NewBook(), notebook.Store{Dir: dir})
			defer a.Close()
			tt := ui.NewTester(a.View, 760, 480)
			tt.Type("unchanged 中文")
			n := a.Book.Current()
			version, light := n.State.Buffer.Version(), n.State.Theme.Text
			tt.SetDark(true)
			dark := n.State.Theme.Text
			if dark == light {
				t.Fatal("system dark not followed")
			}
			tt.SetDark(false)
			if n.State.Theme.Text != light {
				t.Fatal("system light not followed")
			}
			a.OpenThemes()
			tt.Frame()
			click(t, tt, "Appearance Dark")
			click(t, tt, "Save settings")
			tt.SetDark(true)
			tt.SetDark(false)
			if n.State.Theme.Text != dark || n.State.Buffer.Version() != version {
				t.Fatal("explicit dark followed system, or appearance edited document")
			}
		})
	}
}

func TestInvalidPreferencesRemainRecoverable(t *testing.T) {
	for _, original := range []string{`{"theme":`, `{"theme":"unknown","mode":"dark"}`, `{"language":"fr"}`, `{"fontFamily":"unknown"}`, `{"fontSize":0}`, `{"lineHeight":9}`} {
		dir := t.TempDir()
		path := filepath.Join(dir, "preferences.json")
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		a := desktop.New(notebook.NewBook(), notebook.Store{Dir: dir})
		tt := ui.NewTester(a.View, 760, 480)
		tt.Type("still editable")
		a.OpenThemes()
		tt.Frame()
		if !strings.Contains(strings.Join(tt.Texts(), " "), "Preferences not loaded (original kept)") {
			t.Fatal("invalid preferences silently discarded")
		}
		tt.Key(0, ui.KeyEscape)
		if err := a.Flush(); err != nil {
			t.Fatal("bad preferences blocked session save", err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != original {
			t.Fatal("cancel or session save overwrote original preferences")
		}
		a.Close()
	}
}

func TestBundledEditorFontPreviewSaveAndRecovery(t *testing.T) {
	for _, name := range []string{"JetBrains Mono", "IBM Plex Mono", "Source Code Pro"} {
		t.Run(name, func(t *testing.T) {
			store := notebook.Store{Dir: t.TempDir()}
			b := notebook.NewBook()
			a := desktop.New(b, store)
			defer a.Close()
			tt := ui.NewTester(a.View, 760, 620)
			tt.Type("font trial 中文 😀\nkeep undo")
			n := b.Current()
			before, version, family := b.Snapshot(), n.State.Buffer.Version(), n.State.Font.Family
			choose := func() {
				t.Helper()
				tt.Key(ui.Cmd, ui.KeyComma)
				click(t, tt, "Editor")
				click(t, tt, "Editor font")
				click(t, tt, name)
			}
			choose()
			if n.State.Font.Family != family || !tt.HasText(name+" ▾") {
				t.Fatal("font preview changed the editor before saving or did not select the face")
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if !reflect.DeepEqual(before, b.Snapshot()) || n.State.Font.Family != family {
				t.Fatal("cancelling font preview changed document/selection/scroll or font")
			}
			choose()
			path := filepath.Join(store.Dir, "preferences.json")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			click(t, tt, "Save settings")
			if n.State.Font.Family != family || !strings.Contains(strings.Join(tt.Texts(), " "), "Settings not saved") {
				t.Fatal("failed font save applied a draft or hid the error")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			click(t, tt, "Save settings")
			tt.Frame()
			if n.State.Font.Family != "Yohaku "+name || n.State.Buffer.Version() != version {
				t.Fatal("saved font not applied, or font application edited source", n.State.Font)
			}
			registered := false
			for _, f := range ui.FontFamilies() {
				registered = registered || f.Name == "Yohaku "+name && f.Registered && f.Monospaced
			}
			if !registered {
				t.Fatal("selected font is not a real registered monospaced face")
			}
			other := desktop.New(notebook.NewBook(), store)
			defer other.Close()
			ui.NewTester(other.View, 760, 620)
			if other.Book.Current().State.Font.Family != n.State.Font.Family {
				t.Fatal("font not restored from preferences")
			}
			tt.Key(ui.Cmd, ui.KeyComma)
			click(t, tt, "Editor")
			click(t, tt, "Restore default")
			click(t, tt, "Save settings")
			tt.Frame()
			if n.State.Font.Family != family {
				t.Fatal("editor reset did not restore system font")
			}
			n.State.Run(editor.Undo)
			if strings.Contains(n.State.Buffer.String(), "font trial") {
				t.Fatal("font changes lost original undo history")
			}
		})
	}
}

func TestSettingsEditorLanguageAndRecovery(t *testing.T) {
	dir := t.TempDir()
	store := notebook.Store{Dir: dir}
	// The previous theme-only format must still inherit editor defaults.
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"theme":"paper","mode":"light"}`), 0600); err != nil {
		t.Fatal(err)
	}
	b := notebook.NewBook()
	b.Current().Title = "Settings" // A user title must never be translated.
	a := desktop.New(b, store)
	defer a.Close()
	tt := ui.NewTester(a.View, 760, 480)
	tt.Type("first\nsecond\nthird")
	n := b.Current()
	if n.State.Font.Size != 13 || n.State.LineHeight != 1.65 {
		t.Fatal("old preference file lost editor defaults")
	}
	tt.Scroll(450, 300, 0, 10000)
	before := b.Snapshot()
	version := n.State.Buffer.Version()
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Editor")
	tt.Scroll(500, 300, 0, 100)
	click(t, tt, "Increase Font size")
	click(t, tt, "Increase Line height")
	tt.Scroll(500, 300, 0, 10000)
	click(t, tt, "Toggle line numbers")
	click(t, tt, "Toggle scroll past end")
	click(t, tt, "Shortcuts")
	if !tt.HasText("Customize commands · native editing keys stay unchanged") {
		t.Fatal("missing shortcut customization explanation")
	}
	if r, ok := tt.Find("Save settings"); !ok || r.X < 24 || r.Y < 24 || r.X+r.W > 736 || r.Y+r.H > 456 {
		t.Fatal("short-window footer escaped the inset dialog", r)
	}
	tt.Scroll(500, 300, 0, 10000)
	if r, ok := tt.Find("Bold (image & text)"); !ok || r.Y < 24 || r.Y+r.H > 456 {
		t.Fatal("shortcut list cannot scroll to its final entry", r)
	}
	tt.Key(ui.Cmd, ui.KeyN)
	a.NewRichNote() // Native menu callbacks must obey the modal as well.
	if len(b.Open) != 1 {
		t.Fatal("settings leaked a note shortcut")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !reflect.DeepEqual(before, b.Snapshot()) || n.State.Font.Size != 13 || n.State.LineHeight != 1.65 || n.State.LineNumbers != editor.LineNumbersOn || n.State.NoScrollPastEnd {
		t.Fatal("cancel changed editor preferences, scroll, or document")
	}
	// Save across pages, including a language switch. No restart is needed.
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "Editor")
	tt.Scroll(500, 300, 0, 100)
	click(t, tt, "Increase Font size")
	click(t, tt, "Increase Line height")
	tt.Scroll(500, 300, 0, 10000)
	click(t, tt, "Toggle line numbers")
	click(t, tt, "Toggle scroll past end")
	click(t, tt, "Appearance")
	click(t, tt, "Language zh-Hans")
	click(t, tt, "Save settings")
	tt.Frame()
	if a.T("Settings") != "设置" || !tt.HasText("添加块") || !tt.HasText("更多") || n.Title != "Settings" {
		t.Fatal("language not applied to interface, or user title translated", tt.Texts())
	}
	if n.State.Font.Size != 14 || n.State.LineNumbers != editor.LineNumbersOff || !n.State.NoScrollPastEnd || n.State.Buffer.Version() != version {
		t.Fatalf("editor preferences/source: font=%v numbers=%v noPastEnd=%v version=%v want=%v", n.State.Font.Size, n.State.LineNumbers, n.State.NoScrollPastEnd, n.State.Buffer.Version(), version)
	}
	// A native window receives this locale through ConfigureNativeUI. Check
	// that MyGo's own catalog is linked too, not just our settings strings.
	tt.SetLocale("zh-Hans")
	tt.Key(ui.Cmd, ui.KeyF)
	if !tt.HasText("查找") {
		t.Fatal("editor search did not receive the Chinese catalog", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("Settings")
	if !tt.HasText("设置…") || !tt.HasText("Settings") {
		t.Fatal("English command aliases or unmodified note title lost", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	other := desktop.New(notebook.NewBook(), store)
	defer other.Close()
	ui.NewTester(other.View, 760, 480)
	if other.T("Settings") != "设置" || other.Book.Current().State.Font.Size != 14 || !other.Book.Current().State.NoScrollPastEnd {
		t.Fatal("saved language/editor settings did not survive restart")
	}
	// Restoring appearance must not discard editor choices from another page.
	tt.Key(ui.Cmd, ui.KeyComma)
	click(t, tt, "恢复本页默认")
	click(t, tt, "保存设置")
	tt.Frame()
	if a.T("Settings") != "Settings" || n.State.Font.Size != 14 {
		t.Fatal("page reset changed another page or failed to restore English")
	}
	n.State.Run(editor.Undo)
	if strings.Contains(n.State.Buffer.String(), "first") {
		t.Fatal("settings lost original undo history")
	}
}
