package main

import (
	"fmt"
	"maps"
	"regexp"
	"sort"

	caution "github.com/nullentropy/caution"

	"switchboard/uiscript"
)

type uiMount struct {
	name  string
	raw   string
	slots map[string]*uiscript.Node
	err   string
	tick  bool
	inbox bool
}

var uiElapsedRef = regexp.MustCompile(`\.elapsed\b`)

var uiSlotNames = map[string]string{
	"header": "header", "board": "board", "tile": "tile", "railRow": "rail row", "empty": "empty screen",
}

func (v *View) loadUI() bool {
	name, raw := v.hub.ActiveUI()
	if name == "" {
		changed := v.ui != nil
		v.ui = nil
		return changed
	}
	if v.ui != nil && v.ui.name == name && v.ui.raw == string(raw) {
		return false
	}
	m := &uiMount{name: name, raw: string(raw)}
	if ui, errs := uiscript.Parse(raw); len(errs) > 0 {
		m.err = errs[0]
		if len(errs) > 1 {
			m.err += fmt.Sprintf(" (and %d more)", len(errs)-1)
		}
	} else {
		m.slots = ui.Slots
		m.tick = uiElapsedRef.MatchString(m.raw)
		m.inbox = uiHasType(ui.Slots["board"], "sb.inbox")
	}
	v.ui = m
	return true
}

func uiHasType(n *uiscript.Node, typ string) bool {
	if n == nil {
		return false
	}
	if n.Type == typ {
		return true
	}
	for _, k := range n.Kids {
		if uiHasType(k, typ) {
			return true
		}
	}
	return false
}

func (v *View) uiSlot(slot string) *uiscript.Node {
	if v.ui == nil {
		return nil
	}
	return v.ui.slots[slot]
}

func (v *View) uiActive() bool { return v.ui != nil && len(v.ui.slots) > 0 }

func (v *View) uiNow() map[string]any {
	w := 0.0
	if v.s != nil {
		w, _ = v.s.Viewport()
	}
	return uiData(v.hub, uiViewState{Selected: v.sel, Goal: v.goal, Project: v.project, Columns: v.cols, Width: w})
}

func uiRowOf(data map[string]any, a *Agent) map[string]any {
	list, _ := data["sessions"].([]any)
	for _, r := range list {
		if m, _ := r.(map[string]any); m != nil && m["id"] == a.Meta.ID {
			return m
		}
	}
	return nil
}

func (v *View) uiRenderSession(slot string, data map[string]any, a *Agent) (*caution.Node, error) {
	row := uiRowOf(data, a)
	if row == nil {
		return nil, fmt.Errorf("no data for session %q", a.Meta.Title)
	}
	return renderUI(v, v, v.uiSlot(slot), data, row)
}

func uiNotice(name, slot, err string) *caution.Node {
	text := fmt.Sprintf("The %s interface could not load, so the default shows: %s", name, err)
	if slot != "" {
		text = fmt.Sprintf("The %s interface's %s failed, so the default shows here: %s", name, uiSlotNames[slot], err)
	}
	return caution.Panel().H(28).Bg("$panelAlt").Radius(6).Border("$stFail", 1).Tip(text).Kids(
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(10), Right: px(10), CenterY: px(0)}).Kids(
			icon(icFail, 12, "$stFail"),
			caution.Label(text).FontSize(12).Color("$inkDim").Clips().Fill(1),
		))
}

func (v *View) refreshUI() {
	if v.ui == nil {
		return
	}
	v.applyUI()
}

func (v *View) applyUI() {
	v.layoutBoard()
	data := v.uiCur
	if data == nil && v.ui != nil {
		data = v.uiNow()
	}
	v.mountHeader(data)
	v.mountEmpty(data)
}

func (v *View) mountHeader(data map[string]any) {
	var note, head *caution.Node
	switch {
	case v.ui == nil:
	case v.ui.err != "":
		note = uiNotice(v.ui.name, "", v.ui.err)
	case v.uiSlot("header") != nil:
		out, err := renderUI(v, v, v.uiSlot("header"), data, nil)
		if err != nil {
			note = uiNotice(v.ui.name, "header", err.Error())
		} else {
			head = out
		}
	}
	v.uiBar.Clear()
	if note != nil {
		v.uiBar.H(40).Kids(note.Anchor(caution.A{Left: px(wallPad), Right: px(wallPad), CenterY: px(0)}))
	} else {
		v.uiBar.H(0)
	}
	if head == nil {
		if !v.uiHeadOn {
			return
		}
		v.uiHeadOn = false
		v.header.Clear()
		v.uiPark.Clear()
		stats := v.statsRow
		if v.focus != nil {
			stats = v.compact
		}
		v.header.H(v.headerH(v.focus != nil)).Kids(v.headBrand, stats, v.headRight)
		return
	}
	v.header.Clear()
	if !v.uiHeadOn {
		v.uiHeadOn = true
		v.uiPark.Kids(v.statsRow, v.compact)
	}
	inset := 0.0
	if v.native {
		inset = 72
	}
	h := v.headerH(false)
	if x, ok := head.Prop("height").(float64); ok && x > 0 {
		h = x
	}
	v.header.H(h).Add(head.Anchor(caution.A{Left: px(inset), Right: px(0), Top: px(0), Bottom: px(0)}))
}

func (v *View) mountEmpty(data map[string]any) {
	old := v.empty
	on := old.Parent() != nil
	old.Remove()
	v.emptyGo.Remove()
	want := v.emptyGo
	if slot := v.uiSlot("empty"); slot != nil {
		if out, err := renderUI(v, v, slot, data, nil); err == nil {
			want = caution.Panel().Anchor(fill()).Kids(out)
		} else {
			want = caution.Panel().Anchor(fill()).Kids(v.emptyGo,
				uiNotice(v.ui.name, "empty", err.Error()).Anchor(caution.A{Left: px(wallPad), Right: px(wallPad), Top: px(wallPad)}))
		}
	}
	v.empty = want
	if on {
		v.field.Add(want)
	}
}

func (v *View) mountBoard(data map[string]any, notes []*caution.Node) {
	slot := v.uiSlot("board")
	if slot == nil {
		v.showBoardNode(v.boardStack, notes)
		return
	}
	out, err := renderUI(v, v, slot, data, nil)
	if err != nil {
		v.showBoardNode(v.boardStack, append([]*caution.Node{uiNotice(v.ui.name, "board", err.Error())}, notes...))
		return
	}
	holder := caution.VStack().Gap(0).Align("stretch").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	if !v.ui.inbox {
		if inbox := v.sections[0].node; inbox.Parent() != nil {
			inbox.Remove()
			holder.Add(caution.VStack().Pad(wallPad).Align("stretch").Kids(inbox))
		}
	}
	holder.Add(out)
	v.wall.Clear()
	v.wall.Add(holder)
}

func (v *View) showBoardNode(n *caution.Node, notes []*caution.Node) {
	if len(notes) > 0 {
		kids := n.Children()
		n.Clear()
		n.Kids(append(notes, kids...)...)
	}
	if n.Parent() != v.wall {
		v.wall.Clear()
		v.wall.Add(n)
	}
}

func (v *View) limitsMoved() bool {
	l := v.hub.Limits()
	if maps.Equal(l, v.uiLimits) {
		return false
	}
	v.uiLimits = l
	return true
}

func (v *View) uiNames() []string {
	names := make([]string, 0, len(v.set.Interfaces))
	for k := range v.set.Interfaces {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (v *View) useUI(name string) {
	v.hub.UseUI(func(s *Settings) { s.Interface = name })
}

func (v *View) uiOpenSession(a *Agent) { v.select_(a); v.openFocus(a) }
func (v *View) uiNewSession()          { v.openNew() }
func (v *View) uiFocusInput()          { v.focusInput() }
func (v *View) uiOpenPalette()         { v.openPalette() }
func (v *View) uiNextWaiting()         { v.nextWaiting() }
func (v *View) uiShowBoard()           { v.closeFocus() }
func (v *View) uiOpenSettings()        { v.openSettings() }
func (v *View) uiInterrupt(a *Agent)   { a.Interrupt() }
func (v *View) uiFilterGoal(goal string) {
	if goal != "" {
		v.filterGoal(goal)
	}
}
func (v *View) uiClearFilter()           { v.clearFilter() }
func (v *View) uiStopSession(a *Agent)   { v.confirmEnd(a) }
func (v *View) uiRemoveSession(a *Agent) { v.confirmRemove(a) }

func (v *View) uiAnswer(a *Agent, allow, always bool) {
	s := a.Snapshot()
	if len(s.Pending) == 0 || len(s.Pending[0].Questions) > 0 || s.Pending[0].Tool == "ExitPlanMode" {
		return
	}
	a.Answer(s.Pending[0].ReqID, allow, always && s.Pending[0].CanAlways, nil)
}

func (v *View) uiMarkGoalDone(goal string) {
	if goal == "" || len(v.hub.goalSessions(goal)) == 0 || v.hub.GoalDone(goal) {
		return
	}
	v.confirmGoalDone(goal, true)
}
