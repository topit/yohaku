package desktop

import (
	"fmt"
	"maps"
	"strings"

	"github.com/egoist/mygo/ui"
)

type appCommand struct {
	id, title, binding string
	block              bool
	run                func(*ui.Context)
}

// IDs are persistence/API keys; translated titles never identify a binding.
func (a *App) appCommands() []appCommand {
	n := a.Book.Current()
	return []appCommand{
		{"settings", "Settings…", "Cmd+Comma", false, func(*ui.Context) { a.OpenSettings() }},
		{"commands", "Command center…", "Cmd+K", false, func(*ui.Context) { a.OpenCommands() }},
		{"note.new", "New block note", "Cmd+N", false, func(*ui.Context) { a.Book.New(); a.focus, a.renameID = true, "" }},
		{"note.rich", "New image & text note", "Cmd+Alt+N", false, func(*ui.Context) { a.NewRichNote() }},
		{"note.close", "Close current note", "Cmd+W", false, func(*ui.Context) { a.Book.Close(a.Book.Active); a.focus, a.renameID = true, "" }},
		{"note.reopen", "Reopen last closed note", "Cmd+Shift+T", false, func(*ui.Context) { a.Book.Reopen(); a.focus, a.renameID = true, "" }},
		{"note.rename", "Rename current note", "Cmd+Shift+R", false, func(*ui.Context) { a.rename() }},
		{"note.import", "Import Heynote files…", "Cmd+Shift+I", false, func(*ui.Context) { a.ImportHeynoteDialog(false) }},
		{"save", "Save now", "Cmd+S", false, func(*ui.Context) { a.RequestSave() }},
		{"navigation.toggle", "Toggle sidebar", "", false, func(*ui.Context) { a.toggleSidebar() }},
		{"block.language", "Change block language…", "Cmd+L", true, func(*ui.Context) { a.OpenCommands(); a.commands.page = "language" }},
		{"block.add", "Add block", "Cmd+Enter", true, func(*ui.Context) { n.AddBlock(); a.focus = true }},
		{"block.before", "Insert block before", "Alt+Enter", true, func(*ui.Context) { n.AddBlockBefore(); a.focus = true }},
		{"block.split", "Split block at cursor", "Cmd+Shift+Enter", true, func(*ui.Context) { n.SplitBlock(); a.focus = true }},
		{"math.copy", "Copy Math result", "Cmd+Shift+C", true, a.copyMath},
	}
}

var bindingKeys = func() map[string]ui.Key {
	m := map[string]ui.Key{"Enter": ui.KeyEnter, "Comma": ui.KeyComma}
	for i := 0; i < 26; i++ {
		m[string(rune('A'+i))] = ui.KeyA + ui.Key(i)
	}
	for i := 0; i < 10; i++ {
		m[string(rune('0'+i))] = ui.Key0 + ui.Key(i)
	}
	return m
}()

func parseBinding(s string) (ui.Modifiers, ui.Key, bool) {
	if s == "" {
		return 0, 0, true
	}
	parts := strings.Split(s, "+")
	var mods ui.Modifiers
	for _, p := range parts[:len(parts)-1] {
		var mod ui.Modifiers
		switch p {
		case "Cmd":
			mod = ui.Cmd
		case "Alt":
			mod = ui.Alt
		case "Shift":
			mod = ui.Shift
		default:
			return 0, 0, false
		}
		if mods&mod != 0 {
			return 0, 0, false
		}
		mods |= mod
	}
	k, ok := bindingKeys[parts[len(parts)-1]]
	return mods, k, ok && (mods&ui.Cmd != 0 || mods == ui.Alt && k == ui.KeyEnter)
}

func bindingText(s string) string {
	return strings.NewReplacer("Cmd+", "⌘", "Alt+", "⌥", "Shift+", "⇧", "Comma", ",").Replace(s)
}

func effectiveBinding(cmd appCommand, overrides map[string]string) string {
	if s, ok := overrides[cmd.id]; ok {
		return s
	}
	return cmd.binding
}

func (a *App) commandBinding(id string) string {
	for _, cmd := range a.appCommands() {
		if cmd.id == id {
			return effectiveBinding(cmd, a.prefs.Bindings)
		}
	}
	return ""
}

func (a *App) bindingTitle(title, fallback string) string {
	for _, cmd := range a.appCommands() {
		if cmd.title == title {
			return bindingText(effectiveBinding(cmd, a.prefs.Bindings))
		}
	}
	return fallback
}

func (a *App) validateBindings(overrides map[string]string) error {
	seen := map[string]string{}
	for _, cmd := range a.appCommands() {
		s := effectiveBinding(cmd, overrides)
		mods, key, ok := parseBinding(s)
		if !ok {
			return fmt.Errorf("%s: %s", a.T("Invalid shortcut"), s)
		}
		if s == "" {
			continue
		}
		// Preserve native editing and OS/window actions. No unmodified text
		// or IME composition key can become an application command.
		letter := rune('A') + rune(key) - rune(ui.KeyA)
		reserved := mods == ui.Cmd && strings.ContainsRune("ABCDEFGHIMQUVXYZ", letter) ||
			mods == ui.Cmd|ui.Shift && strings.ContainsRune("AGHKLUZ", letter) ||
			mods == ui.Cmd|ui.Alt && strings.ContainsRune("FLVZ", letter)
		if reserved {
			return fmt.Errorf("%s: %s", a.T("Reserved editing or system shortcut"), bindingText(s))
		}
		canonical := fmt.Sprintf("%d:%d", mods, key)
		if other := seen[canonical]; other != "" {
			return fmt.Errorf("%s: %s / %s", a.T("Shortcut conflict"), a.T(other), a.T(cmd.title))
		}
		seen[canonical] = cmd.title
	}
	for id := range overrides {
		known := false
		for _, cmd := range a.appCommands() {
			known = known || cmd.id == id
		}
		if !known {
			return fmt.Errorf("unknown command: %s", id)
		}
	}
	return nil
}

func (a *App) blockShortcut(c *ui.Context, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || a.commands.open || a.themes.open || a.heynote.open {
		return false
	}
	for _, cmd := range a.appCommands() {
		s := effectiveBinding(cmd, a.prefs.Bindings)
		mods, key, _ := parseBinding(s)
		if s != "" && ev.Mods == mods && ev.Key == key {
			cmd.run(c)
			return true
		}
	}
	return false
}

func (a *App) setDraftBinding(id, value string) {
	next := maps.Clone(a.themes.draft.Bindings)
	if next == nil {
		next = map[string]string{}
	}
	next[id] = value
	if err := a.validateBindings(next); err != nil {
		a.themes.err = err.Error()
		return
	}
	a.themes.draft.Bindings = next
	a.themes.recording, a.themes.err = "", ""
}

func (a *App) accelerator(id string) string {
	if a.themes.open {
		return ""
	} // Recording must not trigger native menu actions.
	return strings.ReplaceAll(strings.ReplaceAll(a.commandBinding(id), "Cmd+", "CmdOrCtrl+"), "Comma", ",")
}

func (a *App) commandTip(title, id string) string {
	text := a.T(title)
	if key := a.commandBinding(id); key != "" {
		text += " · " + bindingText(key)
	}
	return text
}
