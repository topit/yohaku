// Package desktop renders only the tabs and one native editing canvas.
package desktop

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"yohaku/internal/notebook"
)

type observation struct {
	ID      string
	Version uint64
}

type App struct {
	Book                          *notebook.Book
	Store                         notebook.Store
	Window                        *mygo.Window
	SaveError                     string
	dirty                         bool
	due                           time.Time
	observed                      []observation
	revision                      uint64
	focus                         bool
	renameID, renameText          string
	renameFocus                   bool
	tabScroll                     ui.ScrollState
	tabRevision                   uint64
	tabViewport                   float32
	sidebarVisible                bool
	math                          mathState
	prefs                         preferences
	preferenceError               string
	colors                        *palette
	buttonsSet, buttonsDark       bool
	commands                      commandState
	blockContext                  blockTarget
	plugins                       pluginState
	themes                        themeState
	heynote                       heynoteImportState
	pending                       func(*ui.Context)
	copyNotice                    string
	save                          saveState
	editorRef, renameRef, findRef ui.Handle
	returnFocus                   *ui.Handle
}

func New(book *notebook.Book, store notebook.Store) *App {
	a := &App{Book: book, Store: store, focus: true, dirty: true}
	a.loadPreferences()
	a.reloadPlugins()
	return a
}

func (a *App) observe() bool {
	// Reading is not a document edit. Save positions with the next content
	// or tab change, explicit save, or normal quit, not on every scroll/click.
	var next []observation
	for _, group := range [][]*notebook.Note{a.Book.Open, a.Book.Closed} {
		for _, n := range group {
			if n.Rich != nil {
				next = append(next, observation{n.ID, n.Rich.Version()})
				continue
			}
			next = append(next, observation{n.ID, n.State.Buffer.Version()})
		}
	}
	changed := a.Book.Revision != a.revision || !reflect.DeepEqual(next, a.observed)
	a.revision, a.observed = a.Book.Revision, next
	return changed
}

func (a *App) autosave(c *ui.Context) {
	if a.trackChanges() {
		c.After(0) // The status row was built before observing this frame's input.
	}
	if !a.dirty {
		return
	}
	if a.save.busy {
		// The writer wakes the native window on completion, not a polling
		// timer that would redraw throughout a slow disk operation.
		return
	}
	if a.due.IsZero() {
		a.due = c.Now().Add(750 * time.Millisecond)
	}
	if !c.Now().Before(a.due) {
		a.RequestSave()
		a.due = time.Time{}
		// Saving happens after this frame's status/error rows were built.
		c.After(0)
	}
	if !a.save.busy && a.dirty {
		c.After(a.due.Sub(c.Now()))
	}
}

func (a *App) rename() {
	a.renameID, a.renameText, a.renameFocus = a.Book.Current().ID, a.Book.Current().Title, true
}

func (a *App) shortcuts(c *ui.Context) {
	if a.commands.open || a.themes.open || a.heynote.open {
		return
	}
	for _, cmd := range a.appCommands() {
		if cmd.block {
			continue
		} // Editor input handles these before native keys.
		s := effectiveBinding(cmd, a.prefs.Bindings)
		mods, key, _ := parseBinding(s)
		if s != "" && c.Shortcut(mods, key) {
			cmd.run(c)
			return
		}
	}
}

func (a *App) export() {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.Window, Title: a.T("Export Yohaku recovery session"), DefaultPath: "Yohaku-recovery.json"})
	if err != nil {
		a.SaveError = err.Error()
		return
	}
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(a.Book.Snapshot(), "", "  ")
	if err == nil {
		err = notebook.AtomicWrite(path, data)
	}
	if err != nil {
		a.SaveError = fmt.Sprintf(a.T("Recovery export: %v"), err)
	} else {
		a.Book.Current().Message = a.T("Recovery session exported. The original save error still needs repair.")
	}
}
