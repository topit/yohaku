package desktop

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/notebook"
)

// The title-bar track, tab proportions and paper card follow Mojian's native
// UI, simplified to one note per tab. See THIRD_PARTY_NOTICES.md.
var (
	fade = ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true}

	plusIcon     = symbol(`M8 2v12M2 8h12`)
	closeIcon    = symbol(`M4 4l8 8M12 4l-8 8`)
	noteIcon     = symbol(`M4 2h5l3 3v9H4zM9 2v4h3M6 9h4M6 11h3`)
	settingsIcon = symbol(`M6.5 1.5h3l.5 2 1.5.8 2-.5 1.5 2.6-1.5 1.5v1.8l1.5 1.5-1.5 2.6-2-.5-1.5.8-.5 2h-3l-.5-2-1.5-.8-2 .5-1.5-2.6 1.5-1.5V7.4L1 5.9l1.5-2.6 2 .5L6 3zM10.5 8a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0`)
	moreIcon     = symbol(`M3 8h.01M8 8h.01M13 8h.01`)
	chevronIcon  = symbol(`M4 6l4 4 4-4`)
	copyIcon     = symbol(`M6 6h8v8H6zM10 6V2H2v8h4`)
	imageIcon    = symbol(`M2 2h12v12H2zM2 11l4-4 3 3 2-2 3 3M10 5h.01`)
	sidebarIcon  = symbol(`M2 2h12v12H2zM6 2v12M3.5 5h1M3.5 8h1M3.5 11h1`)
)

func symbol(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"><path d="` + path + `"/></svg>`))
}

func action(c *ui.Context, icon *ui.SVG, label, tip string, size float32) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Center().Radius(6).Label(label).Tooltip(tip)
	if b.Pressed() {
		b.Background(c.Theme().SurfacePressed)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Transition(fade).Children(func() { ui.Icon(c, icon).Size(14, 14).TextColor(c.Theme().TextMuted) })
	return b
}

func (a *App) View(c *ui.Context) {
	a.pollSave(c.Now())
	a.pluginFrame(c)
	a.appearance(c)
	w, _ := c.Size()
	a.sidebarVisible = a.prefs.Sidebar && w >= 960
	p := a.colors
	if a.copyNotice != "" {
		c.AddToast(ui.Toast{ID: "copy", Title: a.T(a.copyNotice), Type: "success", Timeout: 2 * time.Second})
		a.copyNotice = ""
	}
	if a.pending != nil {
		run := a.pending
		a.pending = nil
		run(c)
	}
	a.shortcuts(c)
	a.mathFrame(c)
	ui.Column(c).Fill().LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: p.Wash[0], At: 0},
		{Color: p.Wash[1], At: 0.4},
		{Color: p.Wash[2], At: 0.75},
		{Color: p.Wash[3], At: 1},
	}}).Children(func() {
		a.titleBar(c)
		ui.Row(c.Key("workspace")).Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Stretch).Children(func() {
			if a.sidebarVisible {
				ui.Column(c.Key("sidebar")).Width(216).Shrink(0).MinHeight(0).Padding(8, 0, 8, 8).Children(func() {
					ui.Text(c, a.T("Notes")).FontSize(11).TextColor(p.UI.TextMuted).Padding(4, 10, 12, 10)
					a.tabs(c, true)
				})
			}
			ui.Column(c.Key("canvas")).Label("Editor surface").Grow(1).MinHeight(0).Margin(0, 8, 8, 8).
				Radius(10).Clip().Background(p.Paper).Border(1, p.UI.Border).
				Shadow(0, 1, 3, 0, p.Shadow).Children(func() {
				a.paneHeader(c)
				n := a.Book.Current()
				ui.Box(c.Key("body")).Grow(1).FillWidth().MinHeight(0).Padding(12, 22, 8, 22).Children(func() {
					if n.Rich != nil {
						a.richView(c)
						return
					}
					st := n.State
					st.OnInput = func(ev ui.InputEvent) bool { return a.mathInput(c, n, ev) }
					st.OnHover = func(request editor.HoverRequest) { a.mathHover(n, request) }
					st.RenderCompletions = mathCompletions
					st.ContextMenu = func(m *ui.Menu) { a.blockMenu(n, m) }
					background := n.Presentation.Decorate(st, [2]ui.Color{p.Paper, p.Alternate})
					pref := a.prefs
					// Settings has its own font preview. Do not reflow/clamp the
					// document's scroll until save; cancellation must leave it alone.
					family, _ := editorFont(pref.FontFamily) // Validated when loading/saving preferences.
					st.Font = ui.Font{Family: family, Size: pref.FontSize}
					st.Placeholder = a.T("Leave room for a thought…")
					st.LineHeight, st.NoScrollPastEnd = pref.LineHeight, !pref.ScrollPastEnd
					st.LineNumbers = editor.LineNumbersOn
					if !pref.LineNumbers {
						st.LineNumbers = editor.LineNumbersOff
					}
					st.Theme = &editor.Theme{Background: background, GutterBackground: p.Paper, Text: p.UI.Text, Cursor: p.UI.Accent,
						Selection: p.UI.Selection, LineNumber: p.UI.TextMuted, ActiveLineNumber: p.UI.Text}
					st.Theme.Tokens[editor.TokenStrong].Bold = true
					st.RenderFindBar = func(c *ui.Context, bar ui.Element, f *editor.FindBar) {
						editor.DefaultFindBar(c, bar, f)
						a.rememberInput(c, bar, &a.findRef)
						if bar.FocusWithin() {
							a.returnFocus = &a.findRef
						}
					}
					v := editor.View(c.Key(n.ID), st).Fill().MinHeight(0)
					a.rememberInput(c, v, &a.editorRef)
					if a.focus {
						v.Focus()
						a.focus = false
					}
					if n.PendingScroll != nil && st.ViewportHeight() > 0 {
						st.SetScrollY(*n.PendingScroll)
						n.PendingScroll = nil
						c.After(0)
					}
				})
				if n.Message != "" {
					ui.Row(c).Padding(8, 16).Gap(8).AlignItems(ui.Center).Background(p.Notice).Children(func() {
						ui.Text(c, a.T(n.Message)).FontSize(11).TextColor(p.NoticeText).Grow(1)
						if action(c, closeIcon, a.T("Dismiss"), a.T("Dismiss notice"), 22).Clicked() {
							n.Message = ""
						}
					})
				}
				if a.SaveError != "" {
					ui.Column(c).Padding(10, 16).Gap(6).Background(p.Error).Children(func() {
						ui.Text(c, a.T("Not saved — content is still in memory; closing is blocked until saved.")).FontSize(12).TextColor(p.UI.Danger)
						ui.Text(c, a.SaveError).FontSize(11).TextColor(p.UI.Danger)
						ui.Row(c).Gap(8).Children(func() {
							if ui.Button(c, a.T("Retry save")).Clicked() {
								a.RequestSave()
							}
							if ui.Button(c, a.T("Export recovery copy…")).Clicked() {
								a.export()
							}
						})
					})
				}
				a.statusBar(c)
			})
		})
	})
	a.commandView(c)
	if !a.heynote.open {
		a.themeView(c)
	}
	a.heynoteImportView(c)
	// Inputs and block commands execute while building the view. Invalidate
	// old results immediately, without treating inlays as document mutations.
	a.mathFrame(c)
	a.autosave(c)
}

func (a *App) titleBar(c *ui.Context) {
	bar := c.TitleBar()
	ui.Row(c).Height(44).Padding(0, max(bar.Right, 12), 0, 0).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Box(c).Width(max(bar.Left+16, 16)).Height(44).DragWindow()
		if action(c, sidebarIcon, a.T("Toggle sidebar"), a.T("Toggle sidebar")+" · "+a.T("Narrow windows use horizontal tabs"), 26).Clicked() {
			a.toggleSidebar()
			c.After(0)
		}
		if !a.sidebarVisible {
			a.tabs(c, false)
			a.newNoteButtons(c, false)
		}
		ui.Box(c).Grow(1).MinWidth(16).DragWindow()
		if action(c, settingsIcon, a.T("Settings"), a.commandTip("Settings", "settings"), 26).Clicked() {
			a.OpenSettings()
			c.After(0)
		}
	})
}

func (a *App) newNoteButtons(c *ui.Context, sidebar bool) {
	add := action(c, plusIcon, a.T("New note"), a.commandTip("New note", "note.new"), 26)
	if sidebar {
		add.Size(168, 32).Radius(8)
		if !add.Hovered() && !add.Pressed() {
			add.Background(a.colors.UI.Selection)
		}
	}
	if add.Clicked() {
		a.Book.New()
		a.focus, a.renameID = true, ""
		c.After(0)
	}
	image := action(c, imageIcon, a.T("New image & text note"), a.commandTip("New image & text note", "note.rich"), 26)
	if sidebar {
		image.Size(30, 32).Radius(8)
		if !image.Hovered() && !image.Pressed() {
			image.Background(a.colors.UI.Selection)
		}
	}
	if image.Clicked() {
		a.NewRichNote()
		c.After(0)
	}
}

func (a *App) tabs(c *ui.Context, vertical bool) {
	p := a.colors
	ink, muted, hover, shade := p.UI.Text, p.UI.TextMuted, p.UI.SurfaceHover, p.Shadow
	widths := make([]float32, len(a.Book.Open))
	total := float32(4 + 2*(len(widths)-1))
	for i, n := range a.Book.Open {
		w, _ := c.MeasureText(0, ui.Span{Text: n.Title, Size: 13, Weight: 500})
		widths[i] = min(max(w+70, 112), 214)
		if vertical {
			widths[i] = 204
			total += 28
		} else {
			total += widths[i]
		}
	}
	var track ui.Element
	if vertical {
		track = ui.Scroll(c.Key("vertical-tabs")).Grow(1).FillWidth().MinHeight(0).Role(ui.RoleTabList).Label("Notes sidebar").TrackScroll(&a.tabScroll)
	} else {
		track = ui.ScrollHorizontal(c).Basis(total).Shrink(1).MinWidth(0).Height(32).Radius(10).
			Background(p.Track).Border(0.5, p.UI.Border).
			ClipX().Role(ui.RoleTabList).Label("Tabs").Scrollbars(false).TrackScroll(&a.tabScroll)
	}
	viewport := track.Bounds().W
	if vertical {
		viewport = track.Bounds().H
	}
	if a.tabRevision != a.Book.Revision || a.tabViewport != viewport {
		start := float32(2)
		for i := 0; i < a.Book.Active; i++ {
			if vertical {
				start += 30
			} else {
				start += widths[i] + 2
			}
		}
		x := a.tabScroll.X
		if vertical {
			x = a.tabScroll.Y
		}
		activeSize := widths[a.Book.Active]
		if vertical {
			activeSize = 28
			if a.Book.Active == len(a.Book.Open)-1 {
				activeSize += 40 // Keep the trailing new-note row within reach.
			}
		}
		if start < x {
			x = start
		} else if end := start + activeSize; end > x+viewport {
			x = end - viewport
		}
		if vertical {
			a.tabScroll.Y = max(0, x)
		} else {
			a.tabScroll.X = max(0, x)
		}
		a.tabRevision, a.tabViewport = a.Book.Revision, viewport
	}
	offset := min(a.tabScroll.X, max(total-viewport, 0))
	closeIndex := -1
	track.Children(func() {
		content := ui.Box(c).Padding(2).Gap(2).AlignItems(ui.Center)
		if vertical {
			content.Column().Height(total + 40).FillWidth()
		} else {
			content.Row().Height(32).Width(total)
		}
		content.Children(func() {
			x := float32(2)
			for i, n := range a.Book.Open {
				whole := vertical || viewport <= 0 || x >= offset-0.5
				x += widths[i] + 2
				active := i == a.Book.Active
				tab := ui.Row(c.Key(n.ID)).Height(28).Width(widths[i]).Shrink(0).Padding(0, 5, 0, 9).Gap(7).
					AlignItems(ui.Center).Radius(8).Role(ui.RoleTab).Label(n.Title).Selected(active).FocusRing(false).
					Tooltip(n.Title + "\n" + a.commandTip("Rename tab", "note.rename"))
				tab.ContextMenu(func(m *ui.Menu) { a.tabMenu(n, m) })
				col := muted
				if active {
					tab.Background(p.Tab).Border(0.5, p.UI.Border).Shadow(0, 1, 2, 0, shade)
					col = ink
				} else if tab.Hovered() {
					tab.Background(hover)
				}
				tab.Transition(fade)
				if tab.Clicked() && !active {
					a.Book.Switch(i)
					a.focus, a.renameID = true, ""
				}
				if tab.DoubleClicked() {
					a.Book.Switch(i)
					a.rename()
				}
				tab.Children(func() {
					// As in Mojian, a left-clipped tab is a blank pill, not a
					// confusing fragment of its title or close button.
					if !whole {
						return
					}
					icon := noteIcon
					if n.Rich != nil {
						icon = imageIcon
					}
					ui.Icon(c, icon).Size(13, 13).TextColor(col).Shrink(0)
					if a.renameID == n.ID {
						in := ui.TextInputBase(c, &a.renameText).Grow(1).MinWidth(0).Height(18).FontSize(13).TextColor(ink).Label("Note name")
						a.rememberInput(c, in, &a.renameRef)
						if a.renameFocus {
							in.Focus().SetTextSelection(0, len([]rune(a.renameText)))
							a.renameFocus = false
						}
						if in.Submitted() {
							a.Book.Rename(a.renameText)
							a.renameID, a.focus = "", true
							c.After(0)
						} else if in.Shortcut(0, ui.KeyEscape) {
							a.renameID, a.focus = "", true
						}
					} else {
						weight := 400
						if active {
							weight = 500
						}
						ui.Text(c, n.Title).FontSize(13).FontWeight(weight).TextColor(col).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					}
					if action(c, closeIcon, "Close "+n.Title, a.commandTip("Archive tab", "note.close"), 18).Clicked() {
						closeIndex = i
						a.focus, a.renameID = true, ""
					}
				})
			}
			if vertical {
				ui.Row(c.Key("new-notes")).Width(204).Height(32).Shrink(0).Margin(6, 0, 0, 0).Gap(6).
					AlignItems(ui.Center).Children(func() { a.newNoteButtons(c, true) })
			}
		})
	})
	// Never mutate the slice while the tab rows are being built.
	if closeIndex >= 0 {
		a.Book.Close(closeIndex)
		c.After(0)
	}
}

func (a *App) tabMenu(n *notebook.Note, m *ui.Menu) {
	i := slices.Index(a.Book.Open, n)
	if i < 0 {
		return
	}
	command := ""
	mods, key, _ := parseBinding(a.commandBinding("note.rename"))
	if m.Item(a.T("Rename tab")).Shortcut(mods, key).Chosen() {
		command = "rename"
	}
	m.Separator()
	mods, key, _ = parseBinding(a.commandBinding("note.close"))
	if m.Item(a.T("Close tab")).Shortcut(mods, key).Chosen() {
		command = "close"
	}
	if m.Item(a.T("Close other tabs")).Disabled(len(a.Book.Open) == 1).Chosen() {
		command = "others"
	}
	tailTitle := "Close tabs to the right"
	if a.sidebarVisible {
		tailTitle = "Close tabs below"
	}
	if m.Item(a.T(tailTitle)).Disabled(i == len(a.Book.Open)-1).Chosen() {
		command = "right"
	}
	if command == "" {
		return
	}
	// Resolve the clicked note again and run before building the next frame:
	// menus must never splice Open while tab widths/rows are being iterated.
	a.pending = func(c *ui.Context) {
		i := slices.Index(a.Book.Open, n)
		if i < 0 {
			return
		}
		if command == "rename" {
			a.Book.Switch(i)
			a.rename()
			a.focus = false
		} else {
			if command == "close" {
				a.Book.Close(i)
			} else {
				for j := len(a.Book.Open) - 1; j >= 0; j-- {
					if command == "others" && j != i || command == "right" && j > i {
						a.Book.Close(j)
					}
				}
			}
			a.focus, a.renameID = true, ""
		}
		c.After(0)
	}
}

func (a *App) paneHeader(c *ui.Context) {
	p := a.colors
	muted, hover := p.UI.TextMuted, p.UI.SurfaceHover
	n := a.Book.Current()
	b, _, _ := n.CurrentBlock()
	ui.Row(c).Height(36).Padding(0, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		if n.Rich != nil {
			a.richTools(c)
			return
		}
		ui.Box(c).Grow(1)
		if b.Language == "Math" {
			r := a.currentMath()
			tip := a.commandTip("Copy Math result", "math.copy")
			if r.Copy != "" {
				tip += "\n" + r.Copy
			} else if r.Error != "" {
				tip = r.Error
			}
			if action(c, copyIcon, "Copy result", tip, 26).Disabled(r.Copy == "").Clicked() {
				a.copyMath(c)
				a.focus = true
			}
		}
		add := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(6).Label("+ Block").Tooltip(a.commandTip("Add block", "block.add"))
		if add.Hovered() {
			add.Background(hover)
		}
		add.Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, plusIcon).Size(12, 12).TextColor(muted)
				ui.Text(c, a.T("Add block")).FontSize(11).TextColor(muted)
			})
		})
		if add.Clicked() {
			n.AddBlock()
			a.focus = true
		}
		more := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(6).Label(a.T("Block actions"))
		if more.Hovered() {
			more.Background(hover)
		}
		more.Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.T("More")).FontSize(11).TextColor(muted)
				ui.Icon(c, moreIcon).Size(12, 12).TextColor(muted)
			})
		})
		more.PopoverMenuButton(func(m *ui.Menu) {
			mods, key, _ := parseBinding(a.commandBinding("block.before"))
			if m.Item(a.T("Insert block before")).Shortcut(mods, key).Chosen() {
				n.AddBlockBefore()
				a.focus = true
			}
			mods, key, _ = parseBinding(a.commandBinding("block.split"))
			if m.Item(a.T("Split block at cursor")).Shortcut(mods, key).Chosen() {
				n.SplitBlock()
				a.focus = true
			}
			m.Separator()
			if m.Item(a.T("Delete current block…")).Danger().Chosen() {
				a.OpenCommands()
				a.commands.page = "delete"
			}
		})
	})
}

func (a *App) statusBar(c *ui.Context) {
	muted := a.colors.UI.TextMuted
	n := a.Book.Current()
	b, index, total := n.CurrentBlock()
	status, dot := "Saved locally", ui.Hex("#a1afa1")
	if a.dirty {
		status, dot = "Saving…", ui.Hex("#c6a46b")
	}
	if a.SaveError != "" {
		status, dot = "Save failed", ui.Hex("#c86259")
	}
	ui.Row(c).Height(26).Padding(0, 14).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(4, 4).Radius(2).Background(dot)
		ui.Text(c, a.T(status)).FontSize(10).TextColor(muted)
		if n.Rich == nil && b.Language == "Math" {
			ui.Text(c, a.mathStatus()).FontSize(10).TextColor(muted).SingleLine().MaxWidth(360).Shrink(1).MinWidth(0).ClipX().Tooltip(a.math.err)
		}
		ui.Box(c).Grow(1)
		if n.Rich != nil {
			ui.Text(c, a.T("Image & text")).FontSize(10).TextColor(muted)
			return
		}
		ui.Textf(c, a.T("Block %d / %d"), index+1, total).FontSize(10).TextColor(muted)
		ui.Box(c).Size(1, 11).Background(a.colors.UI.Border).Margin(0, 3)
		language := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(6).Label("Block language").Tooltip(a.commandTip("Change this block's language", "block.language"))
		if language.Hovered() {
			language.Background(a.colors.UI.SurfaceHover)
		}
		language.Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.Language).FontSize(11).TextColor(muted)
				ui.Icon(c, chevronIcon).Size(12, 12).TextColor(muted)
			})
		})
		language.PopoverMenuButton(func(m *ui.Menu) {
			for _, name := range notebook.Languages {
				if m.Item(name).Checked(name == b.Language).Chosen() {
					n.SetLanguage(name)
					a.focus = true
				}
			}
		})
	})
}
