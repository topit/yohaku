package main

import (
	"flag"
	"log"
	"os"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"yohaku/internal/desktop"
	"yohaku/internal/notebook"
)

func main() {
	dir := flag.String("data-dir", "", "Session directory (default: user data directory)")
	flag.Parse()
	if *dir == "" {
		var err error
		*dir, err = notebook.DataDir()
		if err != nil {
			log.Fatal(err)
		}
	}
	mygo.App.SetName("Yohaku")
	mygo.Theme.SetSource(mygo.ThemeSystem)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	store := notebook.Store{Dir: *dir}
	book, loadErr := store.Load()
	mygo.App.WhenReady(func() {
		if loadErr != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Title: "Yohaku cannot restore this library", Message: "Recovery stopped. Your files were kept.", Detail: loadErr.Error() + "\nKeep the entire data folder, including any pending-save files. Repair the library or open a recovery export in a different -data-dir."})
			mygo.App.Quit()
			return
		}
		a := desktop.New(book, store)
		a.ConfigureNativeUI()
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "Yohaku · 余白", Width: 1080, Height: 800, MinWidth: 760, MinHeight: 480,
			TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
			BackgroundColor: "#efe1e6", Content: ui.View(a.View),
		})
		a.Window = w
		quitting := false
		w.OnClose(func(e *mygo.CloseEvent) {
			if runtime.GOOS == "darwin" && !quitting {
				// Keep the editor and its undo history alive. Only an explicit
				// application quit should destroy the macOS window.
				e.PreventDefault()
				a.SaveThen(w.Hide)
				w.Update(func() {})
				return
			}
			if !a.PrepareClose(w.Close) {
				e.PreventDefault()
				w.Update(func() {})
			} else {
				a.Close()
			}
		})
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			if !a.PrepareClose(mygo.App.Quit) {
				e.PreventDefault()
				w.Update(func() {})
			} else {
				quitting = true // Let Quit close the window instead of hiding it.
				a.Close()
			}
		})
		mygo.App.OnSecondInstance(func([]string, string) { w.Show(); w.Focus() })
		if runtime.GOOS == "darwin" {
			mygo.App.OnActivate(func(visible bool) {
				if !visible {
					w.Show()
					w.Focus()
				}
			})
			a.OfferHeynoteImport()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
