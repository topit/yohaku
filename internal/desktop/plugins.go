package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/localplugin"
	"yohaku/internal/notebook"
)

type pluginConfig struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}
type pluginInfo struct {
	manifest localplugin.Manifest
	err      error
}
type pluginResult struct {
	text string
	err  error
}
type pluginJob struct {
	note       *notebook.Note
	version    uint64
	selections []editor.Selection
	start, end int
	generation uint64
}
type pluginState struct {
	catalog    map[string]pluginInfo
	results    chan pluginResult
	cancel     context.CancelFunc
	job        pluginJob
	busy       bool
	generation uint64
}

func (a *App) reloadPlugins() {
	p := &a.plugins
	if p.cancel != nil {
		p.cancel()
	}
	p.generation++
	p.catalog = map[string]pluginInfo{}
	for _, cfg := range a.prefs.Plugins {
		m, err := localplugin.Inspect(cfg.Path)
		p.catalog[cfg.Path] = pluginInfo{m, err}
	}
}

func (a *App) addPlugin(path string) {
	path, err := filepath.Abs(path)
	if err != nil {
		a.themes.err = err.Error()
		return
	}
	if len(a.themes.draft.Plugins) >= 16 {
		a.themes.err = a.T("At most 16 local plugins")
		return
	}
	m, err := localplugin.Inspect(path)
	if err != nil {
		a.themes.err = err.Error()
		return
	}
	for _, cfg := range a.themes.draft.Plugins {
		info := a.plugins.catalog[cfg.Path]
		if cfg.Path == path || info.manifest.ID == m.ID {
			a.themes.err = a.T("Plugin already added")
			return
		}
	}
	if a.plugins.catalog == nil {
		a.plugins.catalog = map[string]pluginInfo{}
	}
	a.plugins.catalog[path] = pluginInfo{manifest: m}
	a.themes.draft.Plugins = append(a.themes.draft.Plugins, pluginConfig{Path: path})
	a.themes.err = ""
}

func (a *App) startPlugin(path string, m localplugin.Manifest, command localplugin.Command) {
	p, n := &a.plugins, a.Book.Current()
	if p.busy {
		n.Message = "A plugin command is already running."
		return
	}
	if n.Rich != nil {
		n.Message = "Plugin text commands apply to block notes only."
		return
	}
	sels := n.State.Selections()
	if len(sels) != 1 {
		n.Message = "Select one range for this plugin command."
		return
	}
	start, end := min(sels[0].Anchor, sels[0].Head), max(sels[0].Anchor, sels[0].Head)
	b, _, _ := n.CurrentBlock()
	if command.Target == "block" {
		start, end = b.Body, b.End
	}
	if start == end && command.Target == "selection" {
		n.Message = "Select text before running this plugin."
		return
	}
	if end-start > localplugin.MaxText {
		n.Message = "Plugin input exceeds 1 MiB."
		return
	}
	input := localplugin.Input{Text: n.State.Buffer.Snapshot().Slice(start, end), Language: b.Language, NoteTitle: n.Title}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.results, p.busy = cancel, make(chan pluginResult, 1), true
	p.job = pluginJob{n, n.State.Buffer.Version(), slices.Clone(sels), start, end, p.generation}
	results, window := p.results, a.Window
	n.Message = "Running plugin…"
	go func() {
		text, err := localplugin.Run(ctx, path, m, command, input)
		results <- pluginResult{text, err}
		if window != nil {
			window.Update(func() {})
		}
	}()
}

func (a *App) pluginFrame(c *ui.Context) {
	p := &a.plugins
	if !p.busy {
		return
	}
	select {
	case result := <-p.results:
		p.cancel()
		p.cancel = nil
		p.busy = false
		job := p.job
		p.job = pluginJob{}
		p.results = nil
		n := job.note
		if job.generation != p.generation {
			n.Message = "Plugin command cancelled."
			return
		}
		if result.err != nil {
			n.Message = a.T("Plugin failed: ") + result.err.Error()
			return
		}
		if a.Book.Current() != n || n.State.Buffer.Version() != job.version || !slices.Equal(n.State.Selections(), job.selections) {
			n.Message = "Note or selection changed; plugin result was not applied."
			return
		}
		if n.State.Buffer.Snapshot().Slice(job.start, job.end) == result.text {
			n.Message = "Plugin made no changes."
			return
		}
		if n.State.Buffer.TryEdit(editor.Edit{Start: job.start, End: job.end, Text: result.text}) {
			at := job.start + len(result.text)
			n.State.Select(at, at)
			n.Message = "Plugin applied · undo with ⌘Z"
		}
		c.After(0)
	default:
	}
}

func (a *App) pluginItems() []commandItem {
	var items []commandItem
	for _, cfg := range a.prefs.Plugins {
		info := a.plugins.catalog[cfg.Path]
		if !cfg.Enabled || info.err != nil {
			continue
		}
		for _, cmd := range info.manifest.Commands {
			var run func(*ui.Context)
			if a.Book.Current().Rich == nil && !a.plugins.busy {
				run = func(*ui.Context) { a.startPlugin(cfg.Path, info.manifest, cmd) }
			}
			items = append(items, commandItem{title: cmd.Title, detail: info.manifest.Name + " · " + cmd.Target, words: "插件 plugin " + info.manifest.ID + " " + cmd.ID, group: "Plugins", run: run})
		}
	}
	return items
}

func (a *App) pluginSettings(c *ui.Context) {
	ui.Text(c, a.T("Only add plugins you trust. They run in this process, not a security sandbox. Enabling trusts future edits in the same folder.")).FontSize(11).TextColor(a.colors.NoticeText).FillWidth()
	ui.Text(c, a.T("Offline TypeScript text commands · no host files/network · one fresh VM per command.")).FontSize(11).TextColor(a.colors.UI.TextMuted).Margin(8, 0)
	ui.Row(c).Gap(8).Margin(8, 0).Children(func() {
		if ui.Button(c, a.T("Add local plugin…")).Clicked() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: a.T("Choose plugin folder"), Directory: true})
			if err != nil {
				a.themes.err = err.Error()
			} else if len(paths) > 0 {
				a.addPlugin(paths[0])
			}
		}
		if ui.Button(c, a.T("Add example")).Clicked() {
			path, err := localplugin.CreateExample(filepath.Join(a.Store.Dir, "plugins"))
			if err != nil {
				a.themes.err = err.Error()
			} else {
				a.addPlugin(path)
			}
		}
	})
	remove := -1
	for i, cfg := range a.themes.draft.Plugins {
		info := a.plugins.catalog[cfg.Path]
		name := info.manifest.Name
		if name == "" {
			name = filepath.Base(cfg.Path)
		}
		ui.Column(c.Key(cfg.Path)).FillWidth().Padding(12).Margin(6, 0).Radius(7).Border(1, a.colors.UI.Border).Gap(6).Children(func() {
			ui.Text(c, name+" · "+info.manifest.Version).FontSize(13).FontWeight(500)
			ui.Text(c, cfg.Path).FontSize(10).FillWidth().TextColor(a.colors.UI.TextMuted)
			if info.err != nil {
				ui.Text(c, info.err.Error()).FontSize(11).FillWidth().TextColor(a.colors.UI.Danger)
			}
			for _, cmd := range info.manifest.Commands {
				ui.Text(c, fmt.Sprintf("%s · %s", cmd.Title, cmd.Target)).FontSize(11)
			}
			ui.Row(c).Gap(8).Children(func() {
				title := "Trust & enable"
				if cfg.Enabled {
					title = "Disable plugin"
				}
				if ui.Button(c, a.T(title)).Label(title + " " + info.manifest.ID).Disabled(info.err != nil && !cfg.Enabled).Clicked() {
					a.themes.draft.Plugins[i].Enabled = !cfg.Enabled
				}
				if ui.Button(c, a.T("Remove registration")).Label("Remove plugin " + info.manifest.ID).Clicked() {
					remove = i
				}
			})
		})
	}
	if remove >= 0 {
		a.themes.draft.Plugins = slices.Delete(a.themes.draft.Plugins, remove, remove+1)
		c.After(0)
	}
	ui.Text(c, a.T("Save to apply. Removal never deletes source files. Commands appear in the command center; rich notes are not supported yet.")).FontSize(11).FillWidth().TextColor(a.colors.UI.TextMuted).Margin(12, 0)
}
