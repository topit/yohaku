// blockspike is an explicitly unsafe feasibility demo, not the Yohaku MVP.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"yohaku/internal/blockspike"
)

type app struct {
	states []*editor.State
	active int
	focus  bool
}

func (a *app) view(c *ui.Context) {
	st := a.states[a.active]
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		st.Run(editor.Find)
	}
	bg := ui.Hex("#faf9f5")
	ui.Column(c).Fill().Background(bg).Children(func() {
		ui.Row(c).Padding(10, 20).Gap(12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(ui.Hex("#e4e2da")).Children(func() {
			ui.Text(c, "yohaku").FontSize(16).TextColor(ui.Hex("#454b46"))
			ui.Box(c).Width(16)
			for i, title := range []string{"Scratchpad", "Another tab"} {
				b := ui.Button(c.Key(title), title).Padding(8, 16).Radius(7)
				if i == a.active {
					b.Background(ui.Hex("#e7eae3"))
				}
				if b.Clicked() {
					a.active, a.focus = i, true
				}
			}
			ui.Box(c).Grow(1)
			ui.Text(c, "SINGLE BUFFER / BLOCK SPIKE").FontSize(10).TextColor(ui.Hex("#73796f"))
		})
		ui.Column(c).Grow(1).FillWidth().Padding(28, 46).Gap(16).Children(func() {
			ui.Text(c, "One continuous canvas").FontSize(13).TextColor(ui.Hex("#687165"))
			st = a.states[a.active]
			blockspike.Decorate(st)
			st.Theme = &editor.Theme{Background: bg, Text: ui.Hex("#323b35")}
			v := editor.View(c.Key(a.active), st).Grow(1).FillWidth().AutoFocus()
			if a.focus {
				v.Focus()
				a.focus = false
			}
		})
		ui.Row(c).Padding(8, 20).Gap(8).BorderWidth(1, 0, 0, 0).BorderColor(ui.Hex("#e4e2da")).Children(func() {
			ui.Text(c, "Feasibility only · delimiters are NOT protected · no autosave").FontSize(11).TextColor(ui.Hex("#866c43"))
			ui.Box(c).Grow(1)
			ui.Text(c, "⌘F search · native MyGo v0.8.3").FontSize(11).TextColor(ui.Hex("#73796f"))
		})
	})
}

func main() {
	a := &app{states: []*editor.State{
		blockspike.NewState(blockspike.Sample),
		blockspike.NewState("⟦yohaku:JSON⟧\n{\n    \"idea\": \"room for the unfinished\",\n    \"native\": true\n}\n\n⟦yohaku:Plain Text⟧\nSwitch back: each tab keeps its own selection, scroll and undo.\n"),
	}}
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu}}))
		mygo.NewWindow(mygo.WindowOptions{Title: "Yohaku — block feasibility spike", Width: 1050, Height: 830, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
