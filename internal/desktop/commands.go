package desktop

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/notebook"
)

type commandState struct {
	target                      *blockTarget
	open                        bool
	page, query                 string
	index                       int
	scroll                      ui.ScrollState
	composing, compositionEnter bool
}

type commandItem struct {
	title, detail, keys, words, group string
	run                               func(*ui.Context)
	score                             int
}

func (a *App) updateWindow() {
	if a.Window != nil {
		a.Window.Update(func() {})
	}
}

func (a *App) rememberInput(c *ui.Context, element ui.Element, ref *ui.Handle) {
	element.Bind(ref)
	if element.Focused() {
		a.returnFocus = ref
	}
}

func (a *App) restoreInput() {
	if a.returnFocus != nil {
		a.returnFocus.Focus()
	} else {
		a.focus = true
	}
}

func (a *App) OpenCommands() {
	if a.themes.open || a.commands.open || a.heynote.open {
		return
	}
	a.commands = commandState{open: true}
	a.updateWindow()
}

func (a *App) commandItems() []commandItem {
	n := a.Book.Current()
	target := a.commands.target
	runBlock := func(run func(*notebook.Note)) {
		if target != nil {
			a.withBlock(*target, run)
		} else {
			run(n)
		}
	}
	if a.commands.page == "language" {
		var items []commandItem
		for _, name := range notebook.Languages {
			words := "语言"
			if name == "Math" {
				words += " 数学 计算"
			}
			items = append(items, commandItem{title: name, detail: "Block language", words: words, run: func(*ui.Context) { runBlock(func(n *notebook.Note) { n.SetLanguage(name) }) }})
		}
		return items
	}
	if a.commands.page == "delete" {
		return []commandItem{{title: "Delete this block", detail: "Remove its contents · undo with ⌘Z", words: "确认 删除", run: func(*ui.Context) { runBlock((*notebook.Note).RemoveBlock) }}}
	}
	var items []commandItem
	for i, note := range a.Book.Open {
		detail := "Switch note"
		if i == a.Book.Active {
			detail = "Current note"
		}
		kind := "Blocks"
		if note.Rich != nil {
			kind = "Image & text"
		}
		// Titles can be identical. Type and tab position disambiguate without
		// scanning note bodies or exposing a snippet in the command picker.
		detail = fmt.Sprintf("%s · %s · %d", a.T(detail), a.T(kind), i+1)
		group, words := "Notes", "笔记 标签 tab note"
		if a.commands.page == "notes" {
			group, words = "Open notes", ""
		}
		items = append(items, commandItem{title: note.Title, detail: detail, group: group, words: words, run: func(*ui.Context) { a.Book.Switch(slices.Index(a.Book.Open, note)) }})
	}
	if a.commands.page == "notes" {
		for i, note := range a.Book.Closed {
			kind := "Blocks"
			if note.Rich != nil {
				kind = "Image & text"
			}
			detail := fmt.Sprintf("%s · %s · %d", a.T("Closed note"), a.T(kind), i+1)
			items = append(items, commandItem{title: note.Title, detail: detail, keys: a.T("Reopen"), group: "Closed notes", run: func(*ui.Context) {
				a.Book.ReopenAt(slices.Index(a.Book.Closed, note))
			}})
		}
		return items
	}
	add := func(title, detail, keys, words string, run func(*ui.Context)) {
		for _, cmd := range a.appCommands() {
			if cmd.title == title {
				run = cmd.run
				break
			}
		}
		items = append(items, commandItem{title: title, detail: detail, keys: a.bindingTitle(title, keys), words: words, group: "Commands", run: run})
	}
	add("View all notes", "All open and closed notes", "›", "所有 全部 查看 笔记 关闭", func(*ui.Context) { a.commands = commandState{open: true, page: "notes"} })
	add("New block note", "A blank page with language blocks", "⌘N", "新建 笔记 普通", func(*ui.Context) { a.Book.New() })
	add("New image & text note", "Write and paste images", "⌘⌥N", "新建 图文 图片", func(*ui.Context) { a.Book.NewRich() })
	add("Rename current note", n.Title, "⌘⇧R", "重命名 改名 标签", func(*ui.Context) { a.rename() })
	add("Close current note", "Archive, not delete · can be reopened", "⌘W", "关闭 归档 标签", func(*ui.Context) { a.Book.Close(a.Book.Active) })
	if len(a.Book.Closed) > 0 {
		add("Reopen last closed note", a.Book.Closed[len(a.Book.Closed)-1].Title, "⌘⇧T", "重开 恢复 归档", func(*ui.Context) { a.Book.Reopen() })
	}
	if n.Rich == nil {
		add("Add block", "After the current block", "⌘↩", "新建 添加 分块", func(*ui.Context) { n.AddBlock() })
		add("Insert block before", "Before the current block", "⌥↩", "新建 添加 分块 前面", func(*ui.Context) { n.AddBlockBefore() })
		add("Split block at cursor", "Keep both sides as separate blocks", "⌘⇧↩", "拆分 分割 分块", func(*ui.Context) { n.SplitBlock() })
		add("Delete current block…", "Confirm before removing its contents", "", "删除 分块", func(*ui.Context) { a.commands = commandState{open: true, page: "delete"} })
		add("Change block language…", "Plain Text, Math, Go, Markdown…", "⌘L", "切换 语言 数学 计算 math", func(*ui.Context) { a.commands = commandState{open: true, page: "language"} })
		add("Select current block", "Body only, without its divider", "⌘A", "选择 全选 当前 块", func(*ui.Context) { b, _, _ := n.CurrentBlock(); n.State.Select(b.Body, b.End) })
		add("Select entire note", "Across all blocks", "⌘⇧A", "选择 全选 整份 所有", func(*ui.Context) { n.State.Run(editor.SelectAll) })
		add("Find text", "Search the current note", "⌘F", "查找 搜索", func(*ui.Context) { n.State.Run(editor.Find) })
		add("Find and replace", "Search the current note", "", "搜索 替换", func(*ui.Context) { n.State.Run(editor.FindReplace) })
		if n.State.Buffer.CanUndo() {
			add("Undo", "Current note", "⌘Z", "撤销", func(*ui.Context) { n.State.Run(editor.Undo) })
		}
		if n.State.Buffer.CanRedo() {
			add("Redo", "Current note", "⌘⇧Z", "重做", func(*ui.Context) { n.State.Run(editor.Redo) })
		}
		if b, _, _ := n.CurrentBlock(); b.Language == "Math" {
			r := a.currentMath()
			detail := "No complete result on this line"
			var run func(*ui.Context)
			if r.Copy != "" {
				detail = r.Copy
				run = func(c *ui.Context) { a.copyMath(c) }
			}
			add("Copy Math result", detail, "⌘⇧C", "复制 数学 计算 结果", run)
		}
	} else {
		add("Paste image or text", "At your original selection", "⌘V", "粘贴 图片 图文", func(c *ui.Context) { a.pasteRich(c.Services(), false) })
		add("Copy entire image & text note", "The destination chooses HTML, RTFD or plain text", "", "复制 整份 图文 图片", func(c *ui.Context) { a.copyRich(c.Services(), n.Rich.Text()) })
		add("Toggle bold", "Your original selection", "⌘B", "加粗 粗体", func(*ui.Context) { n.Rich.ToggleBold() })
		if n.Rich.CanUndo() {
			add("Undo", "Current note", "⌘Z", "撤销", func(*ui.Context) { n.Rich.Undo() })
		}
		if n.Rich.CanRedo() {
			add("Redo", "Current note", "⌘⇧Z", "重做", func(*ui.Context) { n.Rich.Redo() })
		}
	}
	add("Settings…", "Appearance, editor, shortcuts and local data", "⌘,", "设置 偏好 preferences", func(*ui.Context) { a.OpenSettings() })
	add("Choose theme…", "Four quiet themes · light, dark or system", "", "主题 外观 颜色 深色 浅色", func(*ui.Context) { a.OpenThemes() })
	add("Toggle sidebar", "Narrow windows use horizontal tabs", "", "侧边栏 标签 导航 sidebar tabs", func(*ui.Context) { a.toggleSidebar() })
	add("Import default Heynote library…", "Ask permission before reading the default notes folder", "", "自动 导入 默认 heynote", func(*ui.Context) { a.openDefaultHeynoteImport(false) })
	add("Import Heynote files…", "Original .txt files; sources stay unchanged", "⌘⇧I", "导入 笔记 文件", func(*ui.Context) { a.ImportHeynoteDialog(false) })
	add("Import Heynote library…", "A folder of original notes", "", "导入 笔记库 文件夹", func(*ui.Context) { a.ImportHeynoteDialog(true) })
	add("Save now", "All open and archived notes", "⌘S", "保存 立即", func(*ui.Context) { a.RequestSave() })
	if a.SaveError != "" {
		add("Retry save", "Notes are still in memory", "", "重试 保存 失败", func(*ui.Context) { a.RequestSave() })
	}
	add("Export recovery session…", "All open and archived notes · JSON, not a text export", "", "导出 恢复 备份", func(*ui.Context) { a.export() })
	return append(items, a.pluginItems()...)
}

// Title matching is rune-aware and bounded by titles/keywords, never note bodies.
func commandScore(query, text string) int {
	q, s := []rune(strings.ToLower(strings.TrimSpace(query))), []rune(strings.ToLower(text))
	if len(q) == 0 {
		return 0
	}
	at, score, streak := 0, 0, 0
	for i, r := range s {
		for at < len(q) && unicode.IsSpace(q[at]) {
			at++
		}
		if at == len(q) {
			break
		}
		if r == q[at] {
			at++
			streak++
			score += streak * 3
			if i == 0 || !unicode.IsLetter(s[i-1]) {
				score += 8
			}
		} else {
			streak = 0
		}
	}
	if at != len(q) {
		return -1
	}
	return score
}

func (a *App) commandView(c *ui.Context) {
	if !a.commands.open {
		return
	}
	p, state := a.colors, &a.commands
	w, h := c.Size()
	var chosen func(*ui.Context)
	var backdropClicked bool
	ui.DialogBase(c.Key("commands-"+state.page), &state.open, func(back, panel ui.Element) {
		backdropClicked = back.Clicked()
		back.Background(p.Overlay).Justify(ui.Start).Padding(min(84, h/8), 24, 24, 24)
		panel.Width(600).MaxWidth(w-48).MaxHeight(min(460, h-100)).Radius(12).Background(p.Paper).
			Border(1, p.UI.Border).Shadow(0, 10, 40, 0, p.Shadow).Clip().Label("Command center")
		placeholder := "Search notes and commands…"
		if state.page == "language" {
			placeholder = "Choose block language…"
		} else if state.page == "delete" {
			placeholder = "Confirm block deletion · Esc to cancel"
		} else if state.page == "notes" {
			placeholder = "Search all note titles…"
			ui.Row(c).Height(32).Padding(4, 12, 0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				back := ui.ButtonBase(c).Height(26).Padding(0, 6).Radius(6).Label(a.T("Back to command center"))
				if back.Hovered() {
					back.Background(p.UI.SurfaceHover)
				}
				back.Children(func() { ui.Text(c, "‹ "+a.T("Command center")).FontSize(11).TextColor(p.UI.TextMuted) })
				if back.Clicked() {
					chosen = func(*ui.Context) { a.commands = commandState{open: true} }
				}
				ui.Text(c, "›").FontSize(11).TextColor(p.UI.TextMuted)
				ui.Text(c, a.T("All notes")).FontSize(11)
			})
		}
		input := ui.TextInputBase(c.Key("query"), &state.query).Height(52).Padding(0, 18).FontSize(15).FillWidth().
			Placeholder(a.T(placeholder)).Label("Command query").AutoFocus().ObserveInput(func(ev ui.InputEvent) {
			switch ev.Kind {
			case ui.InputCompose:
				state.composing = ev.Text != ""
			case ui.InputText:
				state.composing = false
			case ui.InputKeyDown:
				if ev.Key == ui.KeyEnter {
					state.compositionEnter, state.composing = state.composing, false
				}
			}
		})
		reveal := input.Changed()
		if input.Changed() {
			state.index, state.scroll.Y = 0, 0
		}
		var items []commandItem
		for _, it := range a.commandItems() {
			// Keep English command names searchable in either language. User note
			// titles and dynamic details remain verbatim, even if they match a key.
			if it.group != "Notes" && state.page != "notes" {
				it.words += " " + it.title + " " + a.T(it.title)
				if it.title != "Rename current note" && it.title != "Reopen last closed note" {
					it.detail = a.T(it.detail)
				}
				it.title = a.T(it.title)
			} else {
				it.detail = a.T(it.detail)
			}
			it.score = max(commandScore(state.query, it.title), commandScore(state.query, it.words))
			if it.score >= 0 {
				items = append(items, it)
			}
		}
		if strings.TrimSpace(state.query) != "" {
			slices.SortStableFunc(items, func(a, b commandItem) int { return b.score - a.score })
		}
		if !state.composing {
			if c.Shortcut(0, ui.KeyDown) {
				state.index++
				reveal = true
			}
			if c.Shortcut(0, ui.KeyUp) {
				state.index--
				reveal = true
			}
		}
		state.index = max(0, min(state.index, len(items)-1))
		if input.Submitted() && !state.compositionEnter && len(items) > 0 {
			chosen = items[state.index].run
		}
		state.compositionEnter = false
		ui.Box(c).Height(1).FillWidth().Background(p.UI.Border)
		ui.Scroll(c.Key("matches")).Grow(1).MinHeight(0).Padding(6).TrackScroll(&state.scroll).Children(func() {
			if len(items) == 0 {
				empty := "No matching notes or commands"
				if state.page == "notes" {
					empty = "No matching notes"
				}
				ui.Text(c, a.T(empty)).Padding(20).FontSize(13).TextColor(p.UI.TextMuted)
			}
			for i, it := range items {
				if strings.TrimSpace(state.query) == "" && it.group != "" && (i == 0 || it.group != items[i-1].group) {
					ui.Text(c, a.T(it.group)).FontSize(10).TextColor(p.UI.TextMuted).Padding(7, 12)
				}
				row := ui.ButtonBase(c.Key(i)).Height(44).FillWidth().Padding(0, 12).Radius(8).
					Label(it.title).Tooltip(it.title + "\n" + it.detail).Disabled(it.run == nil)
				if i == state.index {
					row.Background(p.UI.Selection)
					if reveal {
						row.ScrollIntoView()
					}
				} else if row.Hovered() {
					row.Background(p.UI.SurfaceHover)
				}
				if row.Clicked() {
					chosen = it.run
				}
				row.Children(func() {
					ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
							ui.Text(c, it.title).FontSize(13).SingleLine().Ellipsis("…")
							ui.Text(c, it.detail).FontSize(10).TextColor(p.UI.TextMuted).SingleLine().Ellipsis("…")
						})
						ui.Text(c, it.keys).FontSize(11).TextColor(p.UI.TextMuted).Shrink(0)
					})
				})
			}
		})
		footer := "↑↓  Navigate     ↩  Run     Esc  Cancel                 ⌘K  Command center"
		if state.page == "notes" {
			footer = "↑↓  Navigate     ↩  Open note     Esc  Back"
		}
		ui.Text(c, a.T(footer)).FontSize(10).TextColor(p.UI.TextMuted).Padding(10, 18)
	})
	if chosen != nil {
		a.commands = commandState{}
		a.pending = func(c *ui.Context) {
			a.renameID, a.focus = "", true
			chosen(c)
		}
		c.After(0)
	} else if !state.open {
		if state.page == "notes" && !backdropClicked {
			a.commands = commandState{open: true}
		} else {
			a.commands = commandState{}
			a.restoreInput()
		}
		c.After(0)
	}
}
