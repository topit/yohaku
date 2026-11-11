package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type heynoteImportState struct {
	open      bool
	path, err string
}

// OfferHeynoteImport only checks directory metadata. Reading note contents
// requires explicit consent in the dialog; the native macOS launcher calls it.
func (a *App) OfferHeynoteImport() { a.openDefaultHeynoteImport(true) }

func (a *App) openDefaultHeynoteImport(automatic bool) {
	if a.heynote.open || automatic && a.Book.HeynoteImportHandled {
		return
	}
	home, err := os.UserHomeDir()
	path := filepath.Join(home, "Library", "Application Support", "Heynote", "notes")
	if err == nil {
		var info os.FileInfo
		info, err = os.Stat(path)
		if err == nil && !info.IsDir() {
			err = fmt.Errorf("not a directory: %s", path)
		}
	}
	if err != nil {
		if !automatic {
			message := fmt.Sprintf(a.T("Default Heynote library unavailable: %v"), err)
			if a.themes.open {
				a.themes.err = message
			} else {
				a.Book.Current().Message = message
			}
			a.updateWindow()
		}
		return
	}
	a.commands = commandState{}
	a.heynote = heynoteImportState{open: true, path: path}
	a.updateWindow()
}

func (a *App) heynoteImportView(c *ui.Context) {
	if !a.heynote.open {
		return
	}
	p := a.colors
	w, h := c.Size()
	allow := false
	ui.DialogBase(c.Key("heynote-import"), &a.heynote.open, func(back, panel ui.Element) {
		back.Background(p.Overlay).Padding(24)
		panel.Width(min(520, w-48)).MaxHeight(h-48).Radius(12).Background(p.Paper).
			Border(1, p.UI.Border).Shadow(0, 10, 40, 0, p.Shadow).Clip().Label(a.T("Import from Heynote"))
		ui.Scroll(c.Key("import-details")).FillWidth().MinHeight(0).Padding(24).Children(func() {
			ui.Text(c, a.T("Bring your Heynote notes along")).FontSize(19).FontWeight(500)
			ui.Text(c, a.T("Allow Yohaku to read and import this library?")).FontSize(13).Margin(12, 0)
			ui.Text(c, a.heynote.path).FontSize(11).TextColor(p.UI.TextMuted).FillWidth().
				Padding(12).Radius(7).Background(p.Alternate).Tooltip(a.heynote.path)
			ui.Text(c, a.T("Only after you allow: read .txt notes in this folder and its subfolders, and create one tab per file. Sources stay unchanged. Attachments and settings are not imported.")).FontSize(12).FillWidth().Margin(14, 0)
			ui.Text(c, a.T("One-time copy, not background sync. After importing or declining, this prompt will not return. You can start it again in Settings → Data.")).FontSize(11).TextColor(p.UI.TextMuted).FillWidth()
			if a.heynote.err != "" {
				ui.Text(c, a.heynote.err).FontSize(11).TextColor(p.UI.Danger).FillWidth().Margin(12, 0, 0, 0)
			}
		})
		ui.Row(c).Shrink(0).Gap(8).Padding(0, 24, 20, 24).Justify(ui.End).Children(func() {
			if ui.Button(c, a.T("Don't import")).Clicked() {
				a.heynote.open = false
			}
			allow = ui.Button(c, a.T("Allow & import")).Background(p.UI.Selection).Clicked()
		})
	})
	if allow {
		if !a.ImportHeynote([]string{a.heynote.path}) {
			a.heynote.err = a.Book.Current().Message
			c.After(0)
			return
		}
		a.heynote.open = false
	}
	if !a.heynote.open {
		// Consent completion and imported notes share one atomic session write.
		// Failed writes retain both in memory and keep normal quit protection.
		a.Book.HeynoteImportHandled = true
		a.Book.Revision++
		a.RequestSave()
		a.heynote = heynoteImportState{}
		if !a.themes.open {
			a.restoreInput()
		}
		c.After(0)
	}
}

// ImportHeynoteDialog only reads the chosen files. Cancellation leaves the
// current note, focus and session untouched.
func (a *App) ImportHeynoteDialog(directory bool) {
	if a.themes.open || a.heynote.open {
		return
	}
	options := mygo.OpenDialogOptions{
		Parent: a.Window, Title: a.T("Import Heynote files"), ButtonLabel: a.T("Import"),
		Multiple: true,
		Message:  a.T("Each file becomes a new tab. Source files stay unchanged.\nImports text and block languages, not attachments or settings."),
		Filters:  []mygo.FileFilter{{Name: a.T("Heynote notes"), Extensions: []string{"txt"}}},
	}
	if directory {
		options.Title, options.Directory, options.Multiple = a.T("Import Heynote library folder"), true, false
		options.Filters = nil
		options.Message = a.T("Imports non-hidden .txt notes in this folder and its subfolders.\nSource files stay unchanged; attachments and settings are not imported.")
	}
	paths, err := mygo.Dialog.Open(options)
	if err != nil {
		a.Book.Current().Message = fmt.Sprintf(a.T("Cannot open Heynote import: %v"), err)
		if a.Window != nil {
			a.Window.Update(func() {})
		}
		return
	}
	a.ImportHeynote(paths)
}

func (a *App) ImportHeynote(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	count, warnings, err := a.Book.ImportHeynote(paths)
	if err != nil {
		a.Book.Current().Message = fmt.Sprintf(a.T("Heynote import failed — nothing imported: %v"), err)
	} else {
		a.Book.Current().Message = fmt.Sprintf(a.T("Imported %d Heynote note(s). %s"), count, strings.Join(warnings, " "))
		a.focus, a.renameID = true, ""
	}
	if a.Window != nil {
		a.Window.Update(func() {})
	}
	return err == nil
}
