package main

import (
	"slices"
	"strings"

	caution "github.com/nullentropy/caution"
)

type railRow struct {
	node, bg, title, agent, line *caution.Node
	icon                         *stateMark
	lineFX                       glint
}

func (v *View) buildShell() {
	v.railList = caution.VStack().Gap(2).Pad(8).Align("stretch").
		Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	v.rows = map[*Agent]*railRow{}
	v.holder = caution.Panel()
	split := caution.HSplit().Anchor(fill()).
		SplitPos(sidebarWidth(float64(v.set.SidebarW))).SplitMin(sidebarMin, mainMin)
	split.OnSplitResize(func(pos float64) { v.resizeSidebar(split, pos) })
	v.shell = caution.Panel().Bg("$bg").Anchor(fill()).Kids(
		caution.Glass().Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) {}),
		split.Kids(
			caution.Panel().Bg("$panelAlt").Kids(
				caution.Scroll().NoFade().Anchor(fill()).Kids(v.railList),
				caution.Panel().Bg("$edgeSoft").Anchor(caution.A{Right: px(0), Top: px(0), Bottom: px(0)}).W(1),
			),
			v.holder,
		),
	)
}

const (
	sidebarDefault = 264.0
	sidebarMin     = 200.0
	sidebarMax     = 480.0
	mainMin        = 420.0
)

func sidebarWidth(w float64) float64 {
	if w <= 0 {
		return sidebarDefault
	}
	return min(max(w, sidebarMin), sidebarMax)
}

func (v *View) resizeSidebar(split *caution.Node, pos float64) {
	w := sidebarWidth(pos)
	if w != pos {
		split.SplitPos(w)
	}
	go v.hub.UpdateSettings(func(s *Settings) { s.SidebarW = int(w) })
}

func (v *View) layoutRail() {
	if v.focus == nil {
		return
	}
	v.railList.Clear()
	d := v.hub.Dispatcher
	if v.rows[d] == nil {
		v.rows[d] = v.newRow(d)
	}
	v.railList.Add(v.rows[d].node)
	v.railList.Add(caution.Panel().H(6))
	v.updateRow(d, d.Snapshot())
	if f := v.railFilter(); f != nil {
		v.railList.Add(f)
		v.railList.Add(caution.Panel().H(4))
	}
	list, head := v.railOrder()
	cur := ""
	for _, a := range list {
		if p := projectOf(v.tiles[a].snap.Meta); head && p != cur {
			cur = p
			v.railList.Add(caution.Panel().H(30).NoFade().Kids(
				caution.HStack().Gap(6).Align("center").Anchor(caution.A{Left: px(12), Right: px(12), Bottom: px(4)}).Kids(
					icon(icFolder, 12, "$inkFaint"),
					caution.Label(p).FontSize(11).Weight(650).Color("$inkFaint"),
				)))
		}
		r := v.rows[a]
		if r == nil {
			r = v.newRow(a)
			v.rows[a] = r
		}
		v.railList.Add(r.node)
		if t := v.tiles[a]; t != nil {
			v.updateRow(a, t.snap)
		}
	}
}

func (v *View) railOrder() ([]*Agent, bool) {
	var order []string
	byProject := map[string][]*Agent{}
	for _, a := range v.display() {
		p := projectOf(v.tiles[a].snap.Meta)
		if byProject[p] == nil {
			order = append(order, p)
		}
		byProject[p] = append(byProject[p], a)
	}
	var list []*Agent
	for _, p := range order {
		list = append(list, byProject[p]...)
	}
	return list, len(order) > 1
}

func (v *View) newRow(a *Agent) *railRow {
	r := &railRow{
		icon:  newMark("", 14, "$inkDim"),
		title: caution.Label("").FontSize(13).Weight(600),
		agent: icon("", 12, "$inkFaint"),
		line:  caution.Label("").FontSize(12).Color("$inkDim"),
	}
	r.lineFX.n = r.line
	r.bg = caution.Panel().Radius(8).Anchor(fill())
	r.node = caution.Panel().H(52).NoFade().Kids(
		r.bg,
		caution.Glass().HoverFill("$hover").Anchor(fill()).
			OnPick(func(_, _ float64, _ *caution.Node) { v.switchTo(a) }),
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(12), Right: px(12), Top: px(9)}).
			Kids(r.icon.box, r.title, r.agent),
		r.line.Anchor(caution.A{Left: px(34), Right: px(12), Top: px(30)}),
	)
	if a != v.hub.Dispatcher {
		r.node.Context(v.sessionMenu(a)...)
	}
	return r
}

func (v *View) updateRow(a *Agent, s Snapshot) {
	r := v.rows[a]
	if r == nil || v.focus == nil {
		return
	}
	r.icon.set(stateIcon(s.State), "$"+stateToken(s.State))
	r.title.SetText(clip(s.Meta.Title, 22))
	r.agent.SetText(agentIcon(s.Meta.Provider))
	if a == v.hub.Dispatcher {
		r.agent.SetText(icSparkles)
	}
	text, color := statusLine(s)
	r.line.SetText(clip(text, 32)).SetColor(color)
	r.lineFX.set(v.set, clip(text, 32), 12, false, thinking(s))
	if a == v.focus.a {
		r.bg.Bg("$userBg")
	} else {
		r.bg.Bg("$panelAlt")
	}
}

func (v *View) switchTo(a *Agent) {
	if v.focus != nil && v.focus.a == a {
		return
	}
	if a != v.hub.Dispatcher {
		v.select_(a)
	}
	v.openFocus(a)
}

func (v *View) step(d int) {
	if v.focus == nil {
		v.move(d)
		return
	}
	list, _ := v.railOrder()
	next := railStep(v.hub.Dispatcher, list, v.focus.a, d)
	if next == nil {
		return
	}
	if next != v.hub.Dispatcher {
		v.select_(next)
	}
	v.showFocus(next)
}

func railStep(disp *Agent, sessions []*Agent, cur *Agent, d int) *Agent {
	list := append([]*Agent{disp}, sessions...)
	for i, a := range list {
		if a == cur {
			return list[(i+d+len(list))%len(list)]
		}
	}
	return nil
}

func afterRemoval(disp *Agent, sessions []*Agent, removed *Agent) *Agent {
	i := slices.Index(sessions, removed)
	switch {
	case i >= 0 && i+1 < len(sessions):
		return sessions[i+1]
	case i > 0:
		return sessions[i-1]
	}
	return disp
}

func (v *View) railFilter() *caution.Node {
	var parts []string
	if v.goal != "" {
		parts = append(parts, "Goal: "+v.goal)
	}
	if v.project != "" {
		parts = append(parts, "Project: "+v.project)
	}
	if len(parts) == 0 {
		return nil
	}
	text := strings.Join(parts, ", ")
	return caution.Panel().H(40).Bg("$panel").Radius(8).NoFade().Kids(
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(12), Right: px(8), CenterY: px(0)}).Kids(
			icon(icLayers, 12, "$inkFaint"),
			caution.Label(clip(text, 22)).FontSize(12).Color("$inkDim").Fill(1).Tip(text),
			caution.Button("Show all").Tip("Clear the filter and list every session").OnClick(v.clearFilter),
		))
}

func (v *View) clearFilter() {
	v.goal, v.project = "", ""
	if v.projSel != nil {
		v.projSel.SetSelected(0)
	}
	v.layoutBoard()
}
