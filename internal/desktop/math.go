package desktop

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/mathnote"
	"yohaku/internal/notebook"
)

type mathState struct {
	worker              *mathnote.Worker
	noteID              string
	version, generation uint64
	due                 time.Time
	blocks              []mathnote.Block
	pending             bool
	results             []mathnote.Result
	err                 string
	completionVersion   uint64
	completionAt        int
	contextLine         int
	inlaysDirty         bool
	inlayFont           ui.Font
	inlayWidth          float32
	inlayLanguage       string
}

// Close releases the lazily created worker after a successful window close or
// quit. A failed save must not call this: the user can keep editing/retry.
func (a *App) Close() {
	// Native close reaches here only after PrepareClose succeeds. Headless
	// callers may dispose during a write; join it before releasing the store.
	a.save.after = nil
	a.save.pending = nil
	for a.save.busy {
		a.completeSave(<-a.save.results)
	}
	if a.math.worker != nil {
		a.math.worker.Close()
	}
	if a.plugins.busy {
		a.plugins.cancel()
		<-a.plugins.results
		a.plugins = pluginState{}
	}
}

func (a *App) mathFrame(c *ui.Context) {
	defer a.mathCompletion(c)
	defer a.mathInlays(c)
	n, m := a.Book.Current(), &a.math
	if n.Rich != nil {
		if m.worker != nil {
			m.worker.Cancel()
		}
		m.noteID, m.version = "", 0
		m.results, m.pending, m.due = nil, false, time.Time{}
		return
	}
	if m.noteID != n.ID || m.version != n.State.Buffer.Version() {
		m.noteID, m.version = n.ID, n.State.Buffer.Version()
		m.generation++
		m.completionVersion = 0
		n.State.ShowCompletions(0, nil)
		m.results, m.blocks, m.err, m.pending, m.due = nil, nil, "", false, time.Time{}
		if m.worker != nil {
			m.worker.Cancel()
		}
		n.State.SetInlays(nil)
		n.State.GutterMarks = nil
		text := n.State.Buffer.String()
		snapshot := n.State.Buffer.Snapshot()
		for _, b := range n.Blocks() {
			if b.Language == "Math" {
				m.blocks = append(m.blocks, mathnote.Block{Start: b.Body, Text: text[b.Body:b.End]})
				for i, row := range strings.Split(text[b.Body:b.End], "\n") {
					if mathnote.LineVariable(row) != "" {
						n.State.GutterMarks = append(n.State.GutterMarks, editor.GutterMark{Line: snapshot.LineOf(b.Body) + i, Color: a.colors.UI.Accent})
					}
				}
			}
		}
		if len(m.blocks) > 0 {
			m.due = c.Now().Add(180 * time.Millisecond)
		}
		c.After(0)
	}
	if !m.due.IsZero() {
		if c.Now().Before(m.due) {
			c.After(m.due.Sub(c.Now()))
		} else {
			if m.worker == nil {
				m.worker = mathnote.NewWorker()
			}
			m.worker.Submit(m.generation, m.blocks)
			m.pending, m.due = true, time.Time{}
		}
	}
	if !m.pending {
		return
	}
	select {
	case response := <-m.worker.Results:
		if response.Generation != m.generation {
			break // A cancelled job can finish just before cancellation.
		}
		m.pending = false
		if response.Err != nil {
			m.err = response.Err.Error()
		} else {
			m.results = response.Results
			m.inlaysDirty = true
		}
		c.After(0)
	default:
	}
	if m.pending {
		c.After(30 * time.Millisecond)
	}
}

func (a *App) mathInlays(c *ui.Context) {
	n, m := a.Book.Current(), &a.math
	if n.Rich != nil || m.noteID != n.ID || m.pending || !m.due.IsZero() || len(m.results) == 0 {
		return
	}
	family, _ := editorFont(a.prefs.FontFamily)
	font := ui.Font{Family: family, Size: a.prefs.FontSize}
	width := a.editorRef.Bounds(c).W
	if !m.inlaysDirty && m.inlayFont == font && m.inlayWidth == width && m.inlayLanguage == a.prefs.Language {
		return // Scrolling and idle frames do not reshape the result column.
	}
	m.inlaysDirty, m.inlayFont, m.inlayWidth, m.inlayLanguage = false, font, width, a.prefs.Language
	measure := func(text string) float32 {
		w, _ := ui.ShapeText(text, font, 0).Size()
		return w
	}
	space := measure("0") // All bundled editing fonts have monospaced ASCII.
	limit := min(40*space, max(0, width-10*space)*.55)
	var inlays []editor.Inlay
	index := 0
	for _, block := range m.blocks {
		first := index
		for index < len(m.results) && m.results[index].Start <= block.Start+len(block.Text) {
			index++
		}
		rows := m.results[first:index]
		widths := make([]float32, len(rows))
		column := min(24*space, limit)
		for i, r := range rows {
			widths[i] = -1
			if r.Error != "" || r.Written || r.End-r.Start > 256 {
				continue // Long expressions and diagnostics cannot widen the block.
			}
			line := block.Text[r.Start-block.Start : r.End-block.Start]
			if strings.ContainsRune(line, '\t') {
				var expanded strings.Builder
				col := 0
				for _, ch := range line {
					if ch == '\t' {
						spaces := 4 - col%4
						expanded.WriteString(strings.Repeat(" ", spaces))
						col += spaces
					} else {
						expanded.WriteRune(ch)
						col++
					}
				}
				line = expanded.String()
			}
			w := measure(line)
			if w+5*space <= limit && space > 0 {
				widths[i] = w
				column = min(limit, max(column, float32(math.Ceil(float64((w+5*space)/(8*space))))*8*space))
			}
		}
		for i, r := range rows {
			if r.Written {
				continue // An accepted result is ordinary source text, not a hint.
			}
			text, gap := "= "+r.Display, 3*space
			if r.Complete {
				text = r.Display
			}
			if widths[i] >= 0 {
				gap = column - widths[i]
				if !r.Complete {
					gap -= measure("= ")
				}
			}
			if r.Error != "" {
				text, gap = "   · "+a.mathError(r.Error), 0
			}
			inlays = append(inlays, editor.Inlay{Offset: r.End, Text: text, Padding: gap, CursorBefore: true})
		}
	}
	n.State.SetInlays(inlays)
}

// Keep parser internals out of the writing surface; the hover retains the
// original diagnostic without changing the source, caret or undo history.
func (a *App) mathError(detail string) string {
	switch {
	case strings.Contains(detail, "Undefined symbol"):
		return a.T("Unknown variable")
	case strings.Contains(detail, "Undefined function"):
		return a.T("Unknown function")
	case strings.Contains(detail, "not finite"):
		return a.T("No finite result")
	case strings.Contains(detail, "Not supported"), strings.Contains(detail, "Expected a scalar"):
		return a.T("Unsupported expression")
	case strings.Contains(detail, "too complex"):
		return a.T("Expression too complex")
	default:
		return a.T("Check expression")
	}
}

func (a *App) mathHover(n *notebook.Note, request editor.HoverRequest) {
	m := &a.math
	if m.noteID != n.ID || m.version != request.Version || m.version != n.State.Buffer.Version() || m.pending || !m.due.IsZero() {
		return
	}
	for _, r := range m.results {
		if r.Error != "" && request.Offset >= r.Start && request.Offset <= r.End {
			n.State.ShowHover(r.Start, r.End, a.mathError(r.Error)+"\n"+r.Error, nil)
			return
		}
	}
}

func (a *App) mathCompletion(c *ui.Context) {
	n, m := a.Book.Current(), &a.math
	if n.Rich != nil || !n.State.Focused() || a.commands.open || a.themes.open {
		return
	}
	r := a.currentMath()
	sels := n.State.Selections()
	if !r.Complete || len(sels) != 1 || sels[0].Anchor != r.End || sels[0].Head != r.End {
		m.completionAt = -1
		return
	}
	if m.completionVersion == m.version && m.completionAt == r.End {
		return
	}
	m.completionVersion, m.completionAt = m.version, r.End
	insert := r.Copy
	if text := n.State.Buffer.String(); r.End > 0 && text[r.End-1] == '=' {
		insert = " " + insert
	}
	n.State.ShowCompletions(r.End, []editor.Completion{{Label: r.Display, Insert: insert, Detail: a.T("Tab to insert result"), Kind: editor.CompletionConstant}})
	c.After(0)
}

// Math offers a result, not a code symbol: keep the editor's completion
// behavior, but omit its constant-kind icon (π).
func mathCompletions(c *ui.Context, panel ui.Element, m *editor.CompletionMenu) {
	t, font := c.Theme(), m.Font()
	panel.Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
		Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.2)).Padding(t.Space(1)).
		MinWidth(t.Space(60)).MaxWidth(t.Space(150))
	panel.Children(func() {
		for i, it := range m.Items() {
			row := m.Item(i).Row().Justify(ui.Start).Gap(t.Space(2)).AlignItems(ui.Center).
				Padding(t.Space(0.5), t.Space(1.5)).Radius(t.Radius - 2).Height(m.LineHeight() + 2)
			if i == m.Selected() {
				row.Background(t.Accent.Alpha(0.18))
			}
			row.Children(func() {
				ui.Text(c, it.Label).Font(font.Family).FontSize(font.Size).TextColor(m.Theme().Text).SingleLine().Shrink(1)
				ui.Text(c, it.Detail).FontSize(font.Size - 1).TextColor(t.TextMuted).SingleLine().Shrink(1).Grow(1).TextAlign(ui.End)
			})
		}
	})
}

// Context targets and result clicks use shaped display coordinates, not
// guessed columns. Copying an inlay must leave the source caret untouched.
func (a *App) mathInput(c *ui.Context, n *notebook.Note, ev ui.InputEvent) bool {
	if a.blockShortcut(c, ev) {
		return true
	}
	if ev.Kind == ui.InputPointerDown {
		line, offset := n.State.HitTest(ev.X, ev.Y)
		if ev.Button == 1 {
			a.math.contextLine = line
			a.captureBlock(n, line)
		}
		m := &a.math
		if ev.Button == 0 && ev.Mods == 0 && offset >= 0 && m.noteID == n.ID && m.version == n.State.Buffer.Version() && !m.pending && m.due.IsZero() {
			for _, r := range m.results {
				if r.End == offset && r.Copy != "" {
					c.Services().WriteClipboard(r.Copy)
					a.copyNotice = "Result copied"
					return true
				}
			}
		}
	}
	// The model's default Alt+Enter remains useful without the desktop, but
	// must not survive a desktop override of Insert block before.
	if ev.Kind == ui.InputKeyDown && ev.Mods == ui.Alt && ev.Key == ui.KeyEnter {
		return false
	}
	return n.HandleInput(ev)
}

func (a *App) mathMenu(n *notebook.Note, m *ui.Menu) {
	line := a.math.contextLine
	name, assigned := n.MathLineVariable(line)
	if name == "" {
		return
	}
	title := a.T("Set line as variable") + " " + name
	if assigned {
		title = a.T("Remove line variable") + " " + name
	}
	if m.Item(title).Chosen() {
		t := a.blockContext
		a.pending = func(*ui.Context) {
			if a.validBlock(t) {
				n.ToggleMathLineVariable(line)
				a.focus = true
			}
		}
	}
}

func (a *App) currentMath() mathnote.Result {
	n, m := a.Book.Current(), &a.math
	if m.noteID != n.ID || m.version != n.State.Buffer.Version() || m.pending || !m.due.IsZero() {
		return mathnote.Result{}
	}
	for _, r := range m.results {
		if at := n.State.Cursor(); at >= r.Start && at <= r.End {
			return r
		}
	}
	return mathnote.Result{}
}

func (a *App) copyMath(c *ui.Context) {
	if r := a.currentMath(); r.Copy != "" {
		c.Services().WriteClipboard(r.Copy)
		c.AddToast(ui.Toast{ID: "copy", Title: a.T("Result copied"), Type: "success", Timeout: 2 * time.Second})
	}
}

func (a *App) mathStatus() string {
	if a.math.pending || !a.math.due.IsZero() {
		return a.T("Calculating…")
	}
	if a.math.err != "" {
		// Keep VM implementation traces out of the compact status bar.
		return strings.Split(a.math.err, " at ")[0]
	}
	return ""
}
