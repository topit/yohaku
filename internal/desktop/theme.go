package desktop

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/notebook"
)

type preferences struct {
	Plugins       []pluginConfig    `json:"plugins,omitempty"`
	Bindings      map[string]string `json:"bindings,omitempty"`
	Sidebar       bool              `json:"sidebar,omitempty"`
	Theme         string            `json:"theme"`
	Mode          string            `json:"mode"`
	Language      string            `json:"language"`
	FontFamily    string            `json:"fontFamily,omitempty"`
	FontSize      float32           `json:"fontSize"`
	LineHeight    float32           `json:"lineHeight"`
	LineNumbers   bool              `json:"lineNumbers"`
	ScrollPastEnd bool              `json:"scrollPastEnd"`
}

var defaultPreferences = preferences{Theme: "yohaku", Mode: "light", Language: "en", FontSize: 13, LineHeight: 1.65, LineNumbers: true, ScrollPastEnd: true}

type palette struct {
	UI                          ui.Theme
	Wash                        [4]ui.Color
	Paper, Alternate, Header    ui.Color
	Tab, Track, Shadow, Overlay ui.Color
	Notice, NoticeText, Error   ui.Color
	SyntaxName, SyntaxValue     ui.Color
}

var themeNames = []struct{ id, name, detail string }{
	{"yohaku", "余白 · Yohaku", "Petal light, a quiet dusk"},
	{"paper", "纸白 · Paper", "Neutral, with nothing extra"},
	{"mist", "雾蓝 · Mist", "A little cool air"},
	{"moss", "青苔 · Moss", "Soft greens, room to think"},
}

// Small immutable color tables; previews never create editors or Math VMs.
var palettes = func() map[string][2]*palette {
	out := map[string][2]*palette{}
	for i, theme := range themeNames {
		var pair [2]*palette
		for d := 0; d < 2; d++ {
			t := *ui.LightTheme()
			paper, alternate, header := "#fffefd", "#f8f6fa", "#faf7fa"
			wash := [4]string{"#f6e7f6", "#efe1e6", "#eee0d3", "#ece1bd"}
			accent := []string{"#92668a", "#657286", "#527caa", "#587f65"}[i]
			switch theme.id {
			case "paper":
				paper, alternate, header = "#fefefe", "#f6f7f8", "#f7f7f8"
				wash = [4]string{"#ebedef", "#eff0f1", "#f2f2f3", "#eeeeef"}
			case "mist":
				paper, alternate, header = "#fbfdff", "#f3f7fc", "#f4f8fc"
				wash = [4]string{"#dce7f5", "#e5edf7", "#edf1f5", "#e8eded"}
			case "moss":
				paper, alternate, header = "#fcfdf9", "#f4f7f1", "#f5f8f1"
				wash = [4]string{"#dfebdf", "#e8f0e2", "#f0f0df", "#eee9d6"}
			}
			if d == 1 {
				t = *ui.DarkTheme()
				paper = []string{"#242027", "#222326", "#1c2632", "#202a23"}[i]
				alternate = []string{"#29252e", "#282a2e", "#222d3a", "#273129"}[i]
				header = []string{"#29232e", "#27282c", "#202b39", "#243027"}[i]
				base := []string{"#211824", "#17181b", "#141e2c", "#152219"}[i]
				wash = [4]string{base, base, paper, base}
				accent = []string{"#d5a9cc", "#aabbd8", "#9bbfe8", "#a3c9aa"}[i]
			}
			t.Background, t.Surface = ui.Hex(paper), ui.Hex(header)
			t.Text, t.TextMuted = ui.Hex("#29272d"), ui.Hex("#77727e")
			if d == 1 {
				t.Text, t.TextMuted = ui.Hex("#ece9ef"), ui.Hex("#b0aab7")
			}
			t.Accent, t.Focus = ui.Hex(accent), ui.Hex(accent)
			t.Selection = t.Accent.Mix(t.Background, 0.76)
			t.SurfaceHover = t.Text.Mix(t.Background, 0.93)
			t.SurfacePressed = t.Text.Mix(t.Background, 0.87)
			t.Border = t.Text.Mix(t.Background, 0.86)
			p := &palette{UI: t, Paper: ui.Hex(paper), Alternate: ui.Hex(alternate), Header: ui.Hex(header),
				Tab: ui.Hex(paper), Track: t.Surface.Alpha(0.6), Shadow: ui.RGBA(20, 15, 25, 0.10),
				Overlay: ui.RGBA(18, 14, 24, 0.18), Notice: ui.Hex("#fbf5e8"), NoticeText: ui.Hex("#85642e"), Error: ui.Hex("#fff0ed")}
			p.SyntaxName = ui.Hex([]string{"#866648", "#76664f", "#74658b", "#7b6847"}[i])
			p.SyntaxValue = ui.Hex([]string{"#50698c", "#536b83", "#4d7187", "#4f7369"}[i])
			if d == 1 {
				p.Shadow, p.Overlay = ui.RGBA(0, 0, 0, 0.3), ui.RGBA(0, 0, 0, 0.45)
				p.Notice, p.NoticeText, p.Error = ui.Hex("#373023"), ui.Hex("#e7ca91"), ui.Hex("#3c2529")
				p.SyntaxName = ui.Hex([]string{"#d3b58f", "#cbb99d", "#c4b2d8", "#d0bd91"}[i])
				p.SyntaxValue = ui.Hex([]string{"#a7bddb", "#acc1d5", "#a2c8d5", "#a4c9ba"}[i])
			}
			for j, color := range wash {
				p.Wash[j] = ui.Hex(color).Mix(p.Paper, 0.6)
			}
			pair[d] = p
		}
		out[theme.id] = pair
	}
	return out
}()

func (p *palette) syntax(t *editor.Theme) {
	// Set every token explicitly: zero colors fall back to MyGo's GitHub theme.
	for kind := range t.Tokens {
		t.Tokens[kind].Color = p.UI.Text
	}
	for _, kind := range []editor.TokenKind{editor.TokenKeyword, editor.TokenHeading, editor.TokenTag, editor.TokenLink} {
		t.Tokens[kind].Color = p.UI.Accent
	}
	for _, kind := range []editor.TokenKind{editor.TokenComment, editor.TokenOperator, editor.TokenPunctuation} {
		t.Tokens[kind].Color = p.UI.TextMuted
	}
	for _, kind := range []editor.TokenKind{editor.TokenType, editor.TokenFunction, editor.TokenMeta} {
		t.Tokens[kind].Color = p.SyntaxName
	}
	for _, kind := range []editor.TokenKind{editor.TokenNumber, editor.TokenConstant, editor.TokenProperty, editor.TokenString,
		editor.TokenEscape, editor.TokenRegexp, editor.TokenAttribute, editor.TokenCode} {
		t.Tokens[kind].Color = p.SyntaxValue
	}
	t.Tokens[editor.TokenInserted].Color = p.UI.Success
	t.Tokens[editor.TokenDeleted].Color = p.UI.Danger
	t.Tokens[editor.TokenInvalid].Color = p.UI.Danger
	t.Tokens[editor.TokenStrong].Bold = true
}

func (p preferences) valid() bool {
	_, ok := palettes[p.Theme]
	fontOK := false
	for i := range editorFonts {
		fontOK = fontOK || editorFonts[i].id == p.FontFamily
	}
	return ok && fontOK && len(p.Plugins) <= 16 && (p.Mode == "light" || p.Mode == "dark" || p.Mode == "system") &&
		(p.Language == "en" || p.Language == "zh-Hans") &&
		p.FontSize >= 12 && p.FontSize <= 24 && p.LineHeight >= 1.2 && p.LineHeight <= 2.4
}

func (a *App) preferencesPath() string { return filepath.Join(a.Store.Dir, "preferences.json") }

func (a *App) toggleSidebar() {
	p := a.prefs
	p.Sidebar = !p.Sidebar
	data, err := json.MarshalIndent(p, "", "  ")
	if err == nil {
		err = notebook.AtomicWrite(a.preferencesPath(), data)
	}
	if err != nil {
		a.Book.Current().Message = a.T("Settings not saved: ") + err.Error()
		return
	}
	a.prefs = p
	a.tabViewport = -1
	// The editor handle is relocated, not replaced with a different buffer.
	a.focus = true
	a.updateWindow()
}

func (a *App) loadPreferences() {
	a.prefs = defaultPreferences
	data, err := os.ReadFile(a.preferencesPath())
	if os.IsNotExist(err) {
		return
	}
	// Missing fields in existing theme-only files inherit the current defaults.
	p := defaultPreferences
	if err == nil {
		err = json.Unmarshal(data, &p)
		if err == nil && !p.valid() {
			err = fmt.Errorf("invalid preferences")
		}
		if err == nil {
			err = a.validateBindings(p.Bindings)
		}
		if err == nil {
			_, err = editorFont(p.FontFamily)
		}
	}
	if err != nil {
		a.preferenceError = "Preferences not loaded (original kept): " + err.Error()
		return
	}
	a.prefs = p
}

func (a *App) appearance(c *ui.Context) {
	pref := a.prefs
	if a.themes.open {
		pref = a.themes.draft
	}
	dark := pref.Mode == "dark" || pref.Mode == "system" && c.Theme().Dark
	d := 0
	if dark {
		d = 1
	}
	a.colors = palettes[pref.Theme][d]
	c.SetTheme(&a.colors.UI)
	if a.Window != nil && (!a.buttonsSet || a.buttonsDark != dark) {
		source := mygo.ThemeLight
		if dark {
			source = mygo.ThemeDark
		}
		a.Window.SetWindowButtonsTheme(source)
		a.buttonsSet, a.buttonsDark = true, dark
	}
}

type themeState struct {
	open                               bool
	draft                              preferences
	err                                string
	page                               string
	shortcutQuery, recording, captured string
}

func (a *App) OpenThemes() {
	a.OpenSettings()
	a.themes.page = "appearance"
}

func (a *App) OpenSettings() {
	if a.themes.open || a.heynote.open {
		return
	}
	a.commands = commandState{}
	a.themes = themeState{open: true, draft: a.prefs, err: a.preferenceError, page: "appearance"}
	a.themes.draft.Bindings = maps.Clone(a.prefs.Bindings)
	a.themes.draft.Plugins = slices.Clone(a.prefs.Plugins)
	if a.Window != nil {
		a.ConfigureNativeUI()
	}
	a.updateWindow()
}

func (a *App) themeView(c *ui.Context) {
	if !a.themes.open {
		return
	}
	p := a.colors
	w, h := c.Size()
	apply := false
	ui.DialogBase(c.Key("themes"), &a.themes.open, func(back, panel ui.Element) {
		// Cover the whole window, but inset the panel on every side. A fixed
		// height and shrinkable scroll body keep the footer inside short windows.
		back.Background(p.Overlay).Padding(24)
		panel.Width(min(720, w-48)).Height(min(530, h-48)).Radius(12).
			Background(p.Header).Border(1, p.UI.Border).Shadow(0, 10, 40, 0, p.Shadow).Clip().Label(a.T("Settings"))
		ui.Row(c).Fill().MinHeight(0).AlignItems(ui.Stretch).Padding(10).Gap(10).Children(func() {
			ui.Column(c).Width(146).Shrink(0).MinHeight(0).Padding(14, 6, 8, 6).Gap(4).Children(func() {
				ui.Text(c, a.T("Settings")).FontSize(15).FontWeight(500).Padding(0, 10)
				ui.Text(c, "Yohaku · 余白").FontSize(11).TextColor(p.UI.TextMuted).Padding(4, 10, 20, 10)
				for _, page := range settingsPages {
					b := ui.ButtonBase(c.Key(page.id)).Height(36).FillWidth().Padding(0, 10).Radius(8).
						Role(ui.RoleTab).Label(a.T(page.title)).Selected(a.themes.page == page.id)
					if a.themes.page == page.id {
						b.Background(p.UI.Selection)
					} else if b.Hovered() {
						b.Background(p.UI.SurfaceHover)
					}
					b.Children(func() { ui.Text(c, a.T(page.title)).FontSize(13).TextColor(p.UI.Text) })
					if b.Clicked() {
						a.themes.page = page.id
						c.After(0)
					}
				}
				ui.Box(c).Grow(1)
				ui.Text(c, a.T("Native · Local")).FontSize(11).TextColor(p.UI.TextMuted).Padding(8, 10)
			})
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(10).Background(p.Paper).Border(1, p.UI.Border).Clip().Children(func() {
				ui.Row(c).Height(64).Shrink(0).Padding(0, 20).AlignItems(ui.Center).Children(func() {
					for _, page := range settingsPages {
						if page.id == a.themes.page {
							ui.Column(c).Grow(1).Gap(4).Children(func() {
								ui.Text(c, a.T(page.title)).FontSize(17).FontWeight(500)
								ui.Text(c, a.T(page.detail)).FontSize(11).TextColor(p.UI.TextMuted)
							})
						}
					}
					if action(c, closeIcon, a.T("Close settings"), a.T("Cancel · Esc"), 26).Clicked() {
						a.themes.open = false
					}
				})
				ui.Scroll(c.Key(a.themes.page)).FillWidth().Grow(1).MinHeight(0).Padding(4, 20, 16, 20).Children(func() {
					switch a.themes.page {
					case "appearance":
						a.appearanceSettings(c, min(720, w-48)-220)
					case "editor":
						a.editorSettings(c)
					case "shortcuts":
						a.shortcutSettings(c)
					case "data":
						a.dataSettings(c)
					case "plugins":
						a.pluginSettings(c)
					}
				})
				err := a.themes.err
				if err == "" {
					err = a.SaveError // Background failures must not hide behind the modal.
				}
				if err != "" {
					ui.Text(c, err).FontSize(11).TextColor(p.UI.Danger).FillWidth().MaxHeight(76).Clip().Tooltip(err).Padding(8, 20)
				}
				ui.Row(c).Shrink(0).Gap(8).Padding(12, 20).AlignItems(ui.Center).Children(func() {
					if a.themes.page == "appearance" || a.themes.page == "editor" || a.themes.page == "shortcuts" {
						if ui.Button(c, a.T("Restore default")).Clicked() {
							a.resetSettingsPage()
							c.After(0)
						}
					}
					ui.Box(c).Grow(1)
					if ui.Button(c, a.T("Cancel")).Clicked() {
						a.themes.open = false
					}
					apply = ui.Button(c, a.T("Save settings")).Clicked()
				})
			})
		})
	})
	if apply {
		data, err := json.MarshalIndent(a.themes.draft, "", "  ")
		if err == nil {
			err = a.validateBindings(a.themes.draft.Bindings)
		}
		if err == nil {
			_, err = editorFont(a.themes.draft.FontFamily)
		}
		if err == nil {
			err = notebook.AtomicWrite(a.preferencesPath(), data)
		}
		if err != nil {
			a.themes.err = a.T("Settings not saved: ") + err.Error()
		} else {
			a.prefs, a.preferenceError, a.themes.open = a.themes.draft, "", false
			a.reloadPlugins()
			if a.Window != nil {
				a.ConfigureNativeUI()
			}
		}
		c.After(0)
	}
	if !a.themes.open {
		a.themes = themeState{}
		if a.Window != nil {
			a.ConfigureNativeUI()
		}
		a.restoreInput()
		c.After(0)
	}
}
