package desktop

import (
	"github.com/egoist/mygo/ui"
	"yohaku/internal/notebook"
)

// A menu belongs to the document revision at the moment of the right click,
// not whichever block happens to contain the caret when its action runs.
type blockTarget struct {
	note    *notebook.Note
	version uint64
	index   int
}

func (a *App) captureBlock(n *notebook.Note, line int) {
	at := n.State.Buffer.Snapshot().LineStart(line)
	index := 0
	for i, b := range n.Blocks() {
		if at >= b.Start {
			index = i
		}
	}
	a.blockContext = blockTarget{n, n.State.Buffer.Version(), index}
}

func (a *App) validBlock(t blockTarget) bool {
	return t.note != nil && a.Book.Current() == t.note && t.note.Rich == nil &&
		t.note.State.Buffer.Version() == t.version && t.index < len(t.note.Blocks())
}

func (a *App) withBlock(t blockTarget, run func(*notebook.Note)) {
	if !a.validBlock(t) {
		a.Book.Current().Message = "Block changed; open the menu again."
		return
	}
	b := t.note.Blocks()[t.index]
	at := t.note.State.Cursor()
	if at < b.Body || at > b.End {
		at = b.Body
	}
	t.note.State.Select(at, at)
	run(t.note)
	a.focus = true
}

func (a *App) blockMenu(n *notebook.Note, m *ui.Menu) {
	t := a.blockContext
	if t.note != n {
		return
	}
	valid := a.validBlock(t)
	m.Separator()
	if m.Item(a.T("Copy block text")).Disabled(!valid).Chosen() {
		a.pending = func(c *ui.Context) {
			if a.validBlock(t) {
				b := n.Blocks()[t.index]
				c.Services().WriteClipboard(n.State.Buffer.Snapshot().Slice(b.Body, b.End))
				a.copyNotice = "Block copied"
			}
		}
	}
	for _, item := range []struct {
		title string
		dir   int
	}{{"Move block up", -1}, {"Move block down", 1}} {
		disabled := !valid || t.index+item.dir < 0 || t.index+item.dir >= len(n.Blocks())
		if m.Item(a.T(item.title)).Disabled(disabled).Chosen() {
			a.pending = func(*ui.Context) { a.withBlock(t, func(n *notebook.Note) { n.MoveBlock(item.dir) }) }
		}
	}
	if m.Item(a.T("Split block at cursor")).Disabled(!valid).Chosen() {
		a.pending = func(*ui.Context) { a.withBlock(t, (*notebook.Note).SplitBlock) }
	}
	for _, item := range []struct{ title, page string }{{"Change block language…", "language"}, {"Delete current block…", "delete"}} {
		if m.Item(a.T(item.title)).Disabled(!valid).Chosen() {
			a.pending = func(*ui.Context) { a.commands = commandState{open: true, page: item.page, target: &t} }
		}
	}
	a.mathMenu(n, m)
}
