package desktop

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var settingsPages = []struct{ id, title, detail string }{
	{"appearance", "Appearance", "Choose your colors and light"},
	{"editor", "Editor", "Make writing feel right"},
	{"shortcuts", "Shortcuts", "Customize commands · native editing keys stay unchanged"},
	{"data", "Data", "Your notes stay on this device"},
	{"plugins", "Plugins", "Trusted local TypeScript commands"},
}

func (a *App) appearanceSettings(c *ui.Context, width float32) {
	p, draft := a.colors, &a.themes.draft
	a.settingRow(c, "Language", "Applies after saving · no restart needed", func() {
		for _, lang := range []struct{ id, name string }{{"zh-Hans", "简体中文"}, {"en", "English"}} {
			b := ui.Button(c, lang.name).Label("Language " + lang.id)
			if draft.Language == lang.id {
				b.Background(p.UI.Selection)
			}
			if b.Clicked() {
				draft.Language = lang.id
			}
		}
	})
	a.settingRow(c, "Notes in sidebar", "Narrow windows use horizontal tabs", func() { ui.Switch(c, &draft.Sidebar).Label(a.T("Notes in sidebar")) })
	ui.Text(c, a.T("Appearance")).FontSize(12).Margin(10, 0, 8, 0)
	ui.Row(c).Gap(8).Children(func() {
		for _, mode := range []struct{ id, name string }{{"system", "System"}, {"light", "Light"}, {"dark", "Dark"}} {
			b := ui.Button(c, a.T(mode.name)).Label("Appearance " + mode.name)
			if draft.Mode == mode.id {
				b.Background(p.UI.Selection)
			}
			if b.Clicked() {
				draft.Mode = mode.id
				c.After(0)
			}
		}
	})
	ui.Text(c, a.T("Theme")).FontSize(12).Margin(12, 0, 8, 0)
	ui.Row(c).Wrap().Gap(10).Children(func() {
		for _, theme := range themeNames {
			d := 0
			if p.UI.Dark {
				d = 1
			}
			sample := palettes[theme.id][d]
			card := ui.ButtonBase(c.Key(theme.id)).Column().AlignItems(ui.Stretch).
				Width((width-10)/2).Padding(8).Gap(6).Radius(9).Background(sample.Header).
				Border(1, p.UI.Border).Label("Theme " + theme.id)
			if draft.Theme == theme.id {
				card.Border(2, p.UI.Accent)
			}
			if card.Clicked() {
				draft.Theme = theme.id
				c.After(0)
			}
			card.Children(func() {
				ui.Text(c, a.T(theme.name)).FontSize(12).TextColor(sample.UI.Text)
				ui.Column(c).Radius(4).Clip().Children(func() {
					ui.Text(c, "1  const space = \"余白\"").FontSize(10).TextColor(sample.UI.Text).Background(sample.Paper).Padding(5).FillWidth()
					ui.Text(c, "1  12 × 3   = 36").FontSize(10).TextColor(sample.UI.TextMuted).Background(sample.Alternate).Padding(5).FillWidth()
				})
			})
		}
	})
	ui.Text(c, a.T("Preview only until you save. Cancel restores your previous settings.")).FontSize(11).TextColor(p.UI.TextMuted).Margin(14, 0, 0, 0)
}

func (a *App) settingRow(c *ui.Context, title, detail string, controls func()) {
	ui.Row(c).FillWidth().Padding(12, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, a.T(title)).FontSize(12)
			if detail != "" {
				ui.Text(c, a.T(detail)).FontSize(11).TextColor(a.colors.UI.TextMuted)
			}
		})
		ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(controls)
	})
	ui.Box(c).Height(1).FillWidth().Background(a.colors.UI.Border)
}

func (a *App) editorSettings(c *ui.Context) {
	d := &a.themes.draft
	ui.Text(c, a.T("Block notes only; existing image & text formatting is unchanged.")).FontSize(11).TextColor(a.colors.UI.TextMuted)
	ui.Column(c).FillWidth().Margin(12, 0).Padding(12).Radius(7).Background(a.colors.Alternate).Children(func() {
		lines := []string{a.T("A little room for a thought"), "const space = 0123456789;", "余白 · Hello, world! 😀"}
		if d.LineNumbers {
			for i := range lines {
				lines[i] = fmt.Sprintf("%d   %s", i+1, lines[i])
			}
		}
		family, _ := editorFont(d.FontFamily)
		ui.Text(c, strings.Join(lines, "\n")).Label("Editor font preview").Font(family).FontSize(d.FontSize).LineHeight(d.LineHeight)
	})
	a.settingRow(c, "Font", "Bundled fonts · works offline", func() {
		name := editorFonts[0].name
		for i := range editorFonts {
			if editorFonts[i].id == d.FontFamily {
				name = editorFonts[i].name
			}
		}
		ui.Button(c, a.T(name)+" ▾").Label("Editor font").Width(174).PopoverMenuButton(func(m *ui.Menu) {
			for i := range editorFonts {
				f := &editorFonts[i]
				if m.Item(a.T(f.name)).Checked(f.id == d.FontFamily).Chosen() {
					if _, err := editorFont(f.id); err != nil {
						a.themes.err = a.T("Cannot load font: ") + err.Error()
					} else {
						d.FontFamily = f.id
					}
				}
			}
		})
	})
	for _, field := range []struct {
		name            string
		value           *float32
		step, low, high float32
	}{{"Font size", &d.FontSize, 1, 12, 24}, {"Line height", &d.LineHeight, 0.05, 1.2, 2.4}} {
		a.settingRow(c, field.name, "", func() {
			if ui.Button(c.Key(field.name+"-"), "−").Label("Decrease " + field.name).Disabled(*field.value <= field.low).Clicked() {
				*field.value = max(field.low, *field.value-field.step)
				c.After(0)
			}
			format := "%.0f"
			if field.name == "Line height" {
				format = "%.2f"
			}
			ui.Text(c, fmt.Sprintf(format, *field.value)).FontSize(12).Width(32).TextAlign(ui.Center)
			if ui.Button(c.Key(field.name+"+"), "+").Label("Increase " + field.name).Disabled(*field.value >= field.high).Clicked() {
				*field.value = min(field.high, *field.value+field.step)
				c.After(0)
			}
		})
	}
	a.settingRow(c, "Show line numbers", "Restart from 1 in each block", func() {
		ui.Switch(c, &d.LineNumbers).Label(a.T("Toggle line numbers"))
	})
	a.settingRow(c, "Scroll past end", "Bring the final line to the top", func() {
		ui.Switch(c, &d.ScrollPastEnd).Label(a.T("Toggle scroll past end"))
	})
	ui.Text(c, a.T("Regular weights bundled under SIL OFL 1.1. Chinese and emoji use system fallback.")).FontSize(11).TextColor(a.colors.UI.TextMuted).Margin(10, 0, 0, 0)
}

func (a *App) resetSettingsPage() {
	d := &a.themes.draft
	if a.themes.page == "appearance" {
		d.Theme, d.Mode, d.Language = defaultPreferences.Theme, defaultPreferences.Mode, defaultPreferences.Language
		d.Sidebar = false
	} else if a.themes.page == "editor" {
		d.FontFamily = defaultPreferences.FontFamily
		d.FontSize, d.LineHeight = defaultPreferences.FontSize, defaultPreferences.LineHeight
		d.LineNumbers, d.ScrollPastEnd = true, true
	} else if a.themes.page == "shortcuts" {
		d.Bindings = nil
		a.themes.recording, a.themes.err = "", ""
	}
}

func (a *App) shortcutSettings(c *ui.Context) {
	ui.TextInputBase(c, &a.themes.shortcutQuery).FillWidth().Height(32).Padding(6).Border(1, a.colors.UI.Border).Radius(6).
		Placeholder(a.T("Search commands…")).Label("Search shortcuts")
	ui.Text(c, a.T("Click a shortcut, then press a combination. Esc cancels; Clear disables it. Save to apply.")).FontSize(11).TextColor(a.colors.UI.TextMuted).Margin(10, 0)
	for _, cmd := range a.appCommands() {
		if commandScore(a.themes.shortcutQuery, cmd.title+" "+a.T(cmd.title)+" "+cmd.id) < 0 {
			continue
		}
		ui.Row(c.Key(cmd.id)).FillWidth().Padding(7, 0).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.T(cmd.title)).FontSize(12).Grow(1).MinWidth(0)
			if a.themes.recording == cmd.id {
				ui.TextInputBase(c.Key("capture"), &a.themes.captured).Width(125).Height(28).Border(1, a.colors.UI.Accent).Radius(5).Padding(4).
					Placeholder(a.T("Press keys…")).Label("Record shortcut").AutoFocus().HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind != ui.InputKeyDown {
						return true
					}
					if ev.Key == ui.KeyEscape {
						a.themes.recording = ""
						return true
					}
					for name, key := range bindingKeys {
						if ev.Key == key {
							value := ""
							if ev.Mods&ui.Cmd != 0 {
								value += "Cmd+"
							}
							if ev.Mods&ui.Alt != 0 {
								value += "Alt+"
							}
							if ev.Mods&ui.Shift != 0 {
								value += "Shift+"
							}
							if ev.Mods & ^(ui.Cmd|ui.Alt|ui.Shift) != 0 {
								break
							}
							a.setDraftBinding(cmd.id, value+name)
							break
						}
					}
					return true
				})
			} else {
				value := bindingText(effectiveBinding(cmd, a.themes.draft.Bindings))
				if value == "" {
					value = a.T("Not set")
				}
				if ui.Button(c, value).Label("Shortcut " + cmd.id).MinWidth(104).Clicked() {
					a.themes.recording = cmd.id
					a.themes.captured = ""
				}
			}
			if ui.Button(c, a.T("Clear")).Label("Clear " + cmd.id).Clicked() {
				a.setDraftBinding(cmd.id, "")
			}
			if ui.Button(c, "↺").Label("Reset " + cmd.id).Tooltip(a.T("Restore default")).Clicked() {
				a.setDraftBinding(cmd.id, cmd.binding)
			}
		})
	}
	groups := []struct {
		title string
		rows  [][2]string
	}{
		{"Editing (read-only)", [][2]string{{"Select current block", "⌘A"}, {"Select entire note", "⌘⇧A"}, {"Insert Math result after =", "Tab"}}},
		{"Editing", [][2]string{{"Find text", "⌘F"}, {"Find and replace", "⌘⌥F"}, {"Undo", "⌘Z"}, {"Redo", "⌘⇧Z"}, {"Cut", "⌘X"}, {"Copy", "⌘C"}, {"Paste", "⌘V"}, {"Bold (image & text)", "⌘B"}}},
	}
	for _, group := range groups {
		rows := make([][2]string, 0, len(group.rows))
		for _, row := range group.rows {
			if commandScore(a.themes.shortcutQuery, row[0]+" "+a.T(row[0])) >= 0 {
				rows = append(rows, row)
			}
		}
		if len(rows) == 0 {
			continue
		}
		ui.Text(c, a.T(group.title)).FontSize(12).TextColor(a.colors.UI.TextMuted).Margin(12, 0, 6, 0)
		for _, row := range rows {
			keys := row[1]
			if runtime.GOOS != "darwin" {
				keys = strings.NewReplacer("⌘", "Ctrl+", "⇧", "Shift+", "⌥", "Alt+").Replace(keys)
				if row[0] == "Find and replace" {
					keys = "Ctrl+H"
				}
			}
			ui.Row(c).FillWidth().Padding(8, 0).Gap(8).Children(func() {
				ui.Text(c, a.T(row[0])).FontSize(12).Grow(1).MinWidth(0)
				ui.Text(c, keys).FontSize(11).TextColor(a.colors.UI.TextMuted).Shrink(0)
			})
		}
	}
	ui.Text(c, a.T("Block shortcuts apply to block notes; bold applies to image & text notes.")).FontSize(11).TextColor(a.colors.UI.TextMuted).Margin(14, 0)
}

func (a *App) dataSettings(c *ui.Context) {
	dir, err := filepath.Abs(a.Store.Dir)
	if err != nil {
		dir = a.Store.Dir
	}
	ui.Text(c, a.T("Local data folder")).FontSize(12).Margin(8, 0)
	ui.Text(c, dir).FontSize(11).TextColor(a.colors.UI.TextMuted).FillWidth()
	ui.Row(c).Gap(8).Margin(10, 0, 4, 0).Children(func() {
		for _, folder := range []struct{ label, path string }{{"Open notes folder", a.Store.NotesDir()}, {"Open data folder", dir}} {
			if ui.Button(c, a.T(folder.label)).Clicked() {
				a.SaveThen(func() {
					if err := mygo.Shell.OpenPath(folder.path); err != nil {
						a.themes.err = a.T("Cannot open folder: ") + err.Error()
					}
				})
			}
		}
	})
	ui.Text(c, a.T("notes/ — One .txt per block note; image & text documents use .rich.json. Titles and filenames are listed in session.json.")).FontSize(11).TextColor(a.colors.UI.TextMuted).FillWidth().Margin(8, 0, 4, 0)
	ui.Text(c, a.T("images/ — Shared image files. Back up the entire data folder, not session.json alone.")).FontSize(11).TextColor(a.colors.UI.TextMuted).FillWidth().Margin(0, 0, 4, 0)
	ui.Text(c, a.T("session.json stores tabs and reading positions; preferences.json stores settings. External editing is not synchronized.")).FontSize(11).TextColor(a.colors.UI.TextMuted).FillWidth().Margin(0, 0, 8, 0)
	a.settingRow(c, "Autosave & close protection", "Always on. Failed saves keep your content and prevent closing.", func() {})
	a.settingRow(c, "Recovery copy", "Open and archived notes, including embedded images", func() {
		if ui.Button(c, a.T("Export…")).Clicked() {
			a.export()
			if a.SaveError != "" {
				a.themes.err = a.SaveError
			}
		}
	})
	a.settingRow(c, "Heynote import", "One-time copy · asks before reading your notes", func() {
		if ui.Button(c, a.T("Import from default folder…")).Clicked() {
			a.openDefaultHeynoteImport(false)
			c.After(0)
		}
	})
	ui.Text(c, a.T("Closing a tab archives it, rather than deleting it. Restore it with the command center or ⌘⇧T.")).FontSize(11).TextColor(a.colors.UI.TextMuted).Margin(16, 0)
	ui.Text(c, a.T("Recovery copies contain notes and images, not appearance preferences.")).FontSize(11).TextColor(a.colors.UI.TextMuted)
}
