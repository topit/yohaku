package desktop

import "github.com/egoist/mygo"

// ConfigureNativeUI is called at launch and after a successful settings save.
// MyGo supplies localized native edit/search/context menus in the same locale.
func (a *App) ConfigureNativeUI() {
	mygo.App.SetLanguages("en", "zh-Hans")
	mygo.App.SetLocale(a.prefs.Language)
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: a.T("File"), Submenu: []*mygo.MenuItem{
			{Label: a.T("New image & text note"), Accelerator: a.accelerator("note.rich"), Click: func(*mygo.MenuItem, *mygo.Window) { a.NewRichNote() }},
			{Label: a.T("Import default Heynote library…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openDefaultHeynoteImport(false) }},
			{Label: a.T("Import Heynote files…"), Accelerator: a.accelerator("note.import"), Click: func(*mygo.MenuItem, *mygo.Window) { a.ImportHeynoteDialog(false) }},
			{Label: a.T("Import Heynote library…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.ImportHeynoteDialog(true) }},
			{Type: mygo.MenuItemSeparator},
			{Label: a.T("Settings…"), Accelerator: a.accelerator("settings"), Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenSettings() }},
		}},
		{Label: a.T("View"), Submenu: []*mygo.MenuItem{
			{Label: a.T("Toggle sidebar"), Accelerator: a.accelerator("navigation.toggle"), Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleSidebar() }},
			{Label: a.T("Command center…"), Accelerator: a.accelerator("commands"), Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenCommands() }},
			{Label: a.T("Choose theme…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenThemes() }},
		}},
		{Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
	}))
}
