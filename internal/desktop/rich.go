package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"yohaku/internal/notebook"
)

func (a *App) NewRichNote() {
	if a.themes.open || a.heynote.open {
		return
	}
	a.Book.NewRich()
	a.focus, a.renameID = true, ""
	if a.Window != nil {
		a.Window.Update(func() {})
	}
}

func (a *App) pasteRich(s ui.Services, plain bool) {
	n := a.Book.Current()
	data := s.ReadClipboardData(notebook.RichFormat, transfer.HTML, transfer.PNG, transfer.Text)
	if err := n.PasteRich(data, plain); err != nil {
		n.Message = a.T("Nothing pasted: ") + err.Error()
	} else {
		n.Message = ""
	}
}

func (a *App) copyRich(s ui.Services, text richtext.Text) bool {
	data, err := notebook.RichClipboard(text)
	if err == nil && a.Window != nil {
		data, err = nativeRichClipboard(data, text)
	}
	if err == nil {
		if a.Window != nil {
			err = mygo.Clipboard.Write(data)
		} else {
			s.WriteClipboardData(data)
		}
	}
	if err != nil {
		a.Book.Current().Message = a.T("Copy failed; text was not cut: ") + err.Error()
		return false
	}
	a.copyNotice = "Copied image & text"
	a.updateWindow()
	return true
}

func (a *App) richView(c *ui.Context) {
	n := a.Book.Current()
	s := c.Services()
	p := a.colors
	v := richtext.EditorBase(c.Key(n.ID), n.Rich, richtext.EditorOptions{
		Label: "Image & text editor", Placeholder: a.T("Write a thought, paste an image…"),
		Font: ui.Font{Size: 15}, Theme: &richtext.Theme{Text: p.UI.Text, Muted: p.UI.TextMuted,
			Caret: p.UI.Accent, Selection: p.UI.Selection, ParagraphSpacing: 0.6},
		ScrollY: &n.RichScroll,
		OnPaste: func(plain bool) bool { a.pasteRich(s, plain); return true },
		OnCopy:  func(t richtext.Text) bool { return a.copyRich(s, t) },
	}).Fill().MinHeight(0)
	a.rememberInput(c, v, &a.editorRef)
	if a.focus {
		v.Focus()
		a.focus = false
	}
}

func (a *App) richTools(c *ui.Context) {
	n, s := a.Book.Current(), c.Services()
	muted := a.colors.UI.TextMuted
	ui.Text(c, a.T("Image & text")).FontSize(11).TextColor(muted)
	ui.Box(c).Grow(1)
	if ui.ButtonBase(c).Label("Bold").Tooltip(a.T("Bold · ⌘B")).Size(24, 24).Center().Children(func() {
		ui.Text(c, "B").FontWeight(700).FontSize(12).TextColor(muted)
	}).Clicked() {
		n.Rich.ToggleBold()
		a.focus = true
	}
	if action(c, imageIcon, "Paste image or text", a.T("Paste clipboard image or text · ⌘V"), 24).Clicked() {
		a.pasteRich(s, false)
		a.focus = true
	}
	if action(c, copyIcon, "Copy entire rich note", a.T("Copy entire note with images · HTML + RTF + plain text"), 24).Clicked() {
		a.copyRich(s, n.Rich.Text())
		a.focus = true
	}
}
