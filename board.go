package main

import (
	"fmt"
	"sort"
	"strings"

	caution "github.com/nullentropy/caution"
)

type section struct {
	key, title string
	head, body *caution.Node
	count      *caution.Node
	node       *caution.Node
	inbox      bool
}

func groupOf(s State) string {
	switch s {
	case NeedsYou:
		return "need"
	case Working, Starting:
		return "work"
	case Ready:
		return "done"
	case Failed:
		return "fail"
	case Interrupted:
		return "int"
	case Waiting:
		return "wait"
	case Background:
		return "bg"
	}
	return "idle"
}

func miniGroup(g string) bool { return g == "done" || g == "idle" }

func (v *View) buildBoard() *caution.Node {
	v.goals = caution.VStack().Gap(8).Align("start")
	v.sections = nil
	for _, sc := range []struct{ key, title string }{
		{"need", "Needs you"}, {"work", "Working"}, {"bg", "Background work"}, {"wait", "Waiting for limit"}, {"fail", "Failed"}, {"int", "Interrupted"}, {"done", "Done"}, {"idle", "Ended"},
	} {
		s := &section{key: sc.key, title: sc.title, inbox: sc.key == "need"}
		s.count = caution.Label("").FontSize(11).Weight(600).Color("$inkFaint")
		s.head = caution.HStack().Gap(8).Align("center").Kids(
			caution.Label(strings.ToUpper(sc.title)).FontSize(11).Weight(650).Color("$inkDim"),
			s.count,
			caution.Panel().Fill(1),
		)
		if s.inbox {
			s.head.Add(hints([][2]string{{"A  " + modLabel + "Y", "allow"}, {"D  " + shiftLabel + modLabel + "D", "deny"}, {shiftLabel + "A  " + shiftLabel + modLabel + "Y", "always"}, {modLabel + "J", "next"}, {"Enter", "open"}}))
		}
		switch {
		case s.inbox:
			s.body = caution.VStack().Gap(8).Align("stretch")
		case miniGroup(sc.key):
			s.body = caution.Grid(gridCols(v.cols + 1)).ColGap(10).RowGap(10)
		default:
			s.body = caution.Grid(gridCols(v.cols)).ColGap(gap).RowGap(gap)
		}
		s.node = caution.VStack().Gap(10).Align("stretch").Kids(s.head, s.body)
		v.sections = append(v.sections, s)
	}
	v.boardStack = caution.VStack().Pad(wallPad).Gap(26).Align("stretch").
		Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	return v.boardStack
}

func (v *View) display() []*Agent {
	var out []*Agent
	for _, g := range []string{"need", "work", "bg", "wait", "fail", "int", "done", "idle"} {
		for _, a := range v.order {
			if t := v.tiles[a]; t != nil && groupOf(t.state) == g && (v.goal == "" || t.snap.Meta.Goal == v.goal) &&
				(t.snap.Meta.Goal == v.goal || !v.hub.GoalDone(t.snap.Meta.Goal)) &&
				(v.project == "" || projectOf(t.snap.Meta) == v.project) {
				out = append(out, a)
			}
		}
	}
	return out
}

func (v *View) layoutBoard() {
	if v.goal != "" && !v.showDone && v.hub.GoalDone(v.goal) {
		v.goal = ""
	}
	disp := v.display()
	v.shown = disp
	v.uiCur = nil
	var tiles map[*Agent]*caution.Node
	var notes []*caution.Node
	if v.uiActive() {
		v.uiCur = v.uiNow()
		if v.uiSlot("tile") != nil {
			tiles = map[*Agent]*caution.Node{}
			first := ""
			for _, a := range disp {
				out, err := v.uiRenderSession("tile", v.uiCur, a)
				if err != nil {
					if first == "" {
						first = err.Error()
					}
					continue
				}
				tiles[a] = out
			}
			if first != "" {
				notes = append(notes, uiNotice(v.ui.name, "tile", first))
			}
		}
	}
	v.boardStack.Clear()
	v.layoutGoals()
	if len(v.goals.Children()) > 0 {
		v.boardStack.Add(v.goals)
	}
	idx := map[*Agent]int{}
	for i, a := range disp {
		idx[a] = i
	}
	for _, s := range v.sections {
		s.body.Clear()
	}
	waiting := 0
	for _, a := range disp {
		if groupOf(v.tiles[a].state) == "need" {
			waiting++
		}
	}
	for _, s := range v.sections {
		n := 0
		for _, a := range disp {
			t := v.tiles[a]
			if groupOf(t.state) != s.key {
				continue
			}
			n++
			t.setBadge(idx[a] + 1)
			switch {
			case s.inbox && n > 5:
			case s.inbox && waiting > 3 && t.row != nil:
				s.body.Add(t.row)
			case s.inbox && t.card != nil:
				s.body.Add(t.card)
			case tiles[a] != nil:
				s.body.Add(tiles[a])
			case miniGroup(s.key):
				s.body.Add(t.mini)
			default:
				s.body.Add(t.node)
			}
		}
		if s.inbox && n > 5 {
			s.body.Add(caution.Panel().H(40).Bg("$panel").Radius(10).Border("$edgeSoft", 1).Kids(
				caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { v.nextWaiting() }),
				caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(16), CenterY: px(0)}).Kids(
					caution.Label(fmt.Sprintf("%d more waiting", n-5)).FontSize(13).Color("$inkDim"), kbd(modLabel+"J")),
			))
		}
		if n > 0 {
			s.count.SetText(fmt.Sprint(n))
			v.boardStack.Add(s.node)
		}
	}
	if v.sel != nil && v.tiles[v.sel] == nil {
		v.sel = nil
	}
	v.markTarget()
	v.mountBoard(v.uiCur, notes)
	v.layoutRail()
}

func (v *View) markTarget() {
	first := true
	for _, a := range v.shown {
		t := v.tiles[a]
		if t.card == nil && t.row == nil {
			continue
		}
		t.setTarget(v.sel == nil && first || v.sel == a)
		first = false
	}
}

func hints(hs [][2]string) *caution.Node {
	row := caution.HStack().Gap(12).Align("center")
	for _, h := range hs {
		row.Add(caution.HStack().Gap(5).Align("center").Kids(kbd(h[0]), caution.Label(h[1]).FontSize(11).Color("$inkFaint")))
	}
	return row
}

type goalRow struct {
	name   string
	states []State
	titles []string
}

func (v *View) goalRows() (open, done []*goalRow) {
	byName := map[string]*goalRow{}
	var names []string
	for _, a := range v.order {
		t := v.tiles[a]
		if t == nil || t.snap.Meta.Goal == "" {
			continue
		}
		g := byName[t.snap.Meta.Goal]
		if g == nil {
			g = &goalRow{name: t.snap.Meta.Goal}
			byName[g.name] = g
			names = append(names, g.name)
		}
		g.states = append(g.states, t.state)
		g.titles = append(g.titles, clip(t.snap.Meta.Title, 18))
	}
	sort.Strings(names)
	for _, name := range names {
		if v.hub.GoalDone(name) {
			done = append(done, byName[name])
		} else {
			open = append(open, byName[name])
		}
	}
	return open, done
}

func (v *View) layoutGoals() {
	v.goals.Clear()
	open, done := v.goalRows()
	var chips []*caution.Node
	var widths []float64
	add := func(n *caution.Node, w float64) {
		chips = append(chips, n.W(w))
		widths = append(widths, w)
	}
	for _, g := range open {
		name := g.name
		need := 0
		for _, st := range g.states {
			if st == NeedsYou {
				need++
			}
		}
		tip := name + "  ·  Show only this goal"
		border := "$edgeSoft"
		if v.goal == name {
			tip, border = name+"  ·  Showing only this goal  ·  Esc shows all", "$accent"
		}
		if need > 0 {
			tip += fmt.Sprintf("  ·  %d %s you", need, map[bool]string{true: "needs", false: "need"}[need == 1])
		}
		tip += "  ·  Right-click for Goal done or Remove"
		title := clip(name, 28)
		count := fmt.Sprint(len(g.states))
		row := caution.HStack().Gap(7).Align("center").Anchor(caution.A{Left: px(12), CenterY: px(0)}).Kids(
			icon(icTarget, 13, "$accent"),
			caution.Label(title).FontSize(13).Weight(620),
			caution.Label(count).FontSize(12).Color("$inkDim"),
		)
		w := 12 + 13 + 7 + textW(title, 13, false) + 7 + textW(count, 12, false) + 8
		if need > 0 {
			row.Add(icon(icHand, 12, "$stNeed"))
			w += 7 + 12
		}
		add(caution.Panel().H(30).Bg("$panel").Radius(15).Border(border, 1).Clips().Kids(
			caution.Glass().HoverFill("$hover").Anchor(fill()).Tip(tip).OnPick(func(_, _ float64, _ *caution.Node) { v.filterGoal(name) }),
			row,
		).Context(
			caution.ContextItem{Title: "Goal done", OnPick: func() { v.markGoalDone(name) }},
			caution.ContextItem{Title: "Remove", OnPick: func() { v.confirmRemoveGoal(name) }},
		), w)
	}
	if len(done) > 0 {
		label := fmt.Sprintf("%d done goals", len(done))
		if len(done) == 1 {
			label = "1 done goal"
		}
		tip, act := "Show done goals", "show"
		if v.showDone {
			tip, act = "Hide done goals", "hide"
		}
		add(caution.Panel().H(30).Radius(15).Border("$edgeSoft", 1).Clips().Kids(
			caution.Glass().HoverFill("$hover").Anchor(fill()).Tip(tip).OnPick(func(_, _ float64, _ *caution.Node) {
				v.showDone = !v.showDone
				v.layoutBoard()
			}),
			caution.HStack().Gap(7).Align("center").Anchor(caution.A{Left: px(12), CenterY: px(0)}).Kids(
				icon(icCheck, 13, "$inkDim"),
				caution.Label(label).FontSize(12).Weight(600).Color("$inkDim"),
				caution.Label(act).FontSize(11).Color("$inkFaint"),
			),
		), 12+13+7+textW(label, 12, false)+7+textW(act, 11, false)+8)
	}
	if v.showDone {
		for _, g := range done {
			name := g.name
			tip, border := name+"  ·  Show this goal's sessions", "$edgeSoft"
			if v.goal == name {
				tip, border = name+"  ·  Showing this goal's sessions  ·  Esc shows all", "$accent"
			}
			tip += "  ·  Right-click to Reopen or Remove"
			title := clip(name, 28)
			count := fmt.Sprint(len(g.states))
			add(caution.Panel().H(30).Radius(15).Border(border, 1).Clips().Kids(
				caution.Glass().HoverFill("$hover").Anchor(fill()).Tip(tip).OnPick(func(_, _ float64, _ *caution.Node) { v.filterGoal(name) }),
				caution.HStack().Gap(7).Align("center").Anchor(caution.A{Left: px(12), CenterY: px(0)}).Kids(
					icon(icCheck, 13, "$inkFaint"),
					caution.Label(title).FontSize(13).Weight(600).Color("$inkDim"),
					caution.Label(count).FontSize(12).Color("$inkFaint"),
				),
			).Context(
				caution.ContextItem{Title: "Reopen", OnPick: func() { go v.hub.SetGoalDone(name, false, true) }},
				caution.ContextItem{Title: "Remove", OnPick: func() { v.confirmRemoveGoal(name) }},
			), 12+13+7+textW(title, 13, false)+7+textW(count, 12, false)+8)
		}
	}
	for _, r := range wrapChips(widths, v.goalsWidth(), 8) {
		v.goals.Add(caution.HStack().Gap(8).Align("center").Kids(chips[r[0]:r[1]]...))
	}
}

func (v *View) goalsWidth() float64 {
	w := 1440.0
	if v.s != nil {
		w, _ = v.s.Viewport()
	}
	return w - 2*wallPad
}

func wrapChips(widths []float64, avail, gap float64) [][2]int {
	var rows [][2]int
	start, used := 0, 0.0
	for i, w := range widths {
		if i > start && used+gap+w > avail {
			rows = append(rows, [2]int{start, i})
			start, used = i, 0
		}
		if i > start {
			used += gap
		}
		used += w
	}
	if start < len(widths) {
		rows = append(rows, [2]int{start, len(widths)})
	}
	return rows
}

func (v *View) markGoalDone(name string) { v.confirmGoalDone(name, false) }

func (v *View) confirmGoalDone(name string, ask bool) {
	var live []string
	for _, a := range v.hub.goalSessions(name) {
		switch s := a.Snapshot(); s.State {
		case Working, Starting, NeedsYou, Waiting, Background:
			live = append(live, s.Meta.Title)
		}
	}
	finish := func() {
		if v.goal == name {
			v.goal = ""
		}
		go v.hub.SetGoalDone(name, true, true)
	}
	if len(live) == 0 && !ask {
		finish()
		return
	}
	text := "Its sessions stay as they are, and the goal can be reopened."
	if len(live) > 0 {
		text = fmt.Sprintf("This stops %s, which %s not finished. Their conversations stay, and the goal can be reopened.", strings.Join(live, ", "), map[bool]string{true: "has", false: "have"}[len(live) == 1])
	}
	v.showDialog(caution.Dialog("Mark "+name+" done?").CardSize(460, 190).Kids(
		caution.Label(text).
			FontSize(13).Color("$inkDim").Wrap().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Mark done").Primary().OnClick(func() { v.closeDialog(); finish() }),
		),
	))
}

func (v *View) confirmRemoveGoal(name string) {
	var titles []string
	for _, a := range v.hub.goalSessions(name) {
		titles = append(titles, a.Snapshot().Meta.Title)
	}
	remove := func(keepWork bool) func() {
		return func() {
			v.closeDialog()
			if v.goal == name {
				v.goal = ""
			}
			go v.hub.RemoveGoal(name, keepWork, true)
		}
	}
	text := fmt.Sprintf("This stops and removes %s. Their conversations stay on disk, and worktrees with nothing unmerged go too.", strings.Join(titles, ", "))
	buttons := []*caution.Node{
		caution.Button("Cancel").OnClick(v.closeDialog),
		caution.Button("Remove goal").Primary().OnClick(remove(true)),
	}
	if left := v.hub.goalLeftovers(name); len(left) > 0 {
		text += " " + strings.Join(left, ". ") + ". Keep worktrees leaves those worktrees and branches in place. Delete worktrees throws that work away."
		buttons = []*caution.Node{
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Delete worktrees").OnClick(remove(false)),
			caution.Button("Keep worktrees").Primary().OnClick(remove(true)),
		}
	}
	v.showDialog(caution.Dialog("Remove "+name+"?").CardSize(520, 240).Kids(
		caution.Label(text).FontSize(13).Color("$inkDim").Wrap().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(buttons...),
	))
}

func (v *View) goalsChanged() {
	v.s.Update(func() {
		if v.goal != "" && len(v.hub.goalSessions(v.goal)) == 0 {
			v.goal = ""
		}
		v.layoutBoard()
		v.refreshSummary()
	})
}

func (v *View) select_(a *Agent) {
	if v.sel != nil && v.tiles[v.sel] != nil {
		v.tiles[v.sel].setSelected(false)
	}
	v.sel = a
	if t := v.tiles[a]; t != nil {
		t.setSelected(true)
		if v.uiActive() {
			v.refreshUI()
		}
		switch {
		case t.state == NeedsYou && t.card != nil:
			t.card.Reveal()
		case miniGroup(groupOf(t.state)):
			t.mini.Reveal()
		default:
			t.node.Reveal()
		}
	}
	v.markTarget()
}

func (v *View) move(d int) {
	if v.focus != nil || len(v.shown) == 0 {
		return
	}
	i := -1
	for j, a := range v.shown {
		if a == v.sel {
			i = j
		}
	}
	switch {
	case i < 0 && d > 0:
		i = 0
	case i < 0:
		i = len(v.shown) - 1
	default:
		i = (i + d + len(v.shown)) % len(v.shown)
	}
	v.select_(v.shown[i])
}

func (v *View) target() *Agent {
	if v.focus != nil {
		return v.focus.a
	}
	if v.sel != nil {
		return v.sel
	}
	for _, a := range v.shown {
		if v.tiles[a].state == NeedsYou {
			return a
		}
	}
	return nil
}

func (v *View) answer(allow, always bool) {
	a := v.target()
	if a == nil {
		return
	}
	s := a.Snapshot()
	if len(s.Pending) == 0 || len(s.Pending[0].Questions) > 0 || s.Pending[0].Tool == "ExitPlanMode" {
		return
	}
	a.Answer(s.Pending[0].ReqID, allow, always && s.Pending[0].CanAlways, nil)
}

func (v *View) nextWaiting() {
	var waiting []*Agent
	for _, a := range v.shown {
		if v.tiles[a].state == NeedsYou {
			waiting = append(waiting, a)
		}
	}
	if len(waiting) == 0 {
		return
	}
	next := waiting[0]
	for i, a := range waiting {
		if a == v.sel && i+1 < len(waiting) {
			next = waiting[i+1]
		}
	}
	v.closeFocus()
	v.select_(next)
}

func (v *View) bindKeys() {
	s := v.s
	for i := 1; i <= 9; i++ {
		n := i
		s.OnKey(fmt.Sprintf("%s+%d", modKey, n), func() {
			if n <= len(v.shown) {
				v.select_(v.shown[n-1])
				v.openFocus(v.shown[n-1])
			}
		})
	}
	s.OnKey(modKey+"+0", v.closeFocus)
	s.OnKey(modKey+"+n", v.openNew)
	s.OnKey(modKey+"+k", v.openPalette)
	s.OnKey(modKey+"+l", v.focusInput)
	s.OnKey(modKey+"+j", v.nextWaiting)
	s.OnKey(modKey+"+.", func() {
		if a := v.target(); a != nil {
			a.Interrupt()
		}
	})
	s.OnKey(modKey+"+e", func() {
		if a := v.target(); a != nil && len(a.Snapshot().Queued) > 0 {
			v.openFocus(a)
			v.focus.editNewest()
		}
	})
	s.OnKey(modKey+"+shift+e", func() {
		switch {
		case v.focus != nil:
			v.focus.sendNow()
		case v.target() != nil:
			go v.target().SendNow("")
		}
	})
	s.OnKey(modKey+"+backspace", func() {
		if a := v.target(); a != nil {
			v.confirmRemove(a)
		}
	})
	for _, k := range []string{"j", "down"} {
		s.OnKey(k, func() { v.step(1) })
	}
	for _, k := range []string{"k", "up"} {
		s.OnKey(k, func() { v.step(-1) })
	}
	for _, k := range []string{"enter", "o"} {
		s.OnKey(k, func() {
			switch {
			case v.askSend != nil:
				v.askSend()
			case v.focus != nil:
				v.focus.composer.Focus()
			case v.sel != nil:
				v.openFocus(v.sel)
			}
		})
	}
	s.OnKey("a", func() { v.answer(true, false) })
	s.OnKey("shift+a", func() { v.answer(true, true) })
	s.OnKey("d", func() { v.answer(false, false) })
	s.OnKey(modKey+"+y", func() { v.answer(true, false) })
	s.OnKey(modKey+"+shift+y", func() { v.answer(true, true) })
	s.OnKey(modKey+"+shift+d", func() { v.answer(false, false) })
	s.OnKey(".", func() {
		if a := v.target(); a != nil {
			a.Interrupt()
		}
	})
	s.OnKey("r", func() {
		if a := v.target(); a != nil {
			v.openRename(a)
		}
	})
	s.OnKey("/", v.openPalette)
	s.OnKey("shift+/", v.openShortcuts)
	s.OnKey("?", v.openShortcuts)
	s.OnKey("shift+?", v.openShortcuts)
	s.OnKey("escape", func() {
		switch {
		case v.focus != nil && v.focus.editing != 0:
			v.focus.cancelEdit()
		case v.focus != nil:
			v.closeFocus()
		case v.goal != "":
			v.filterGoal(v.goal)
		case v.project != "":
			v.filterProject(v.project)
		case v.sel != nil:
			v.tiles[v.sel].setSelected(false)
			v.sel = nil
		}
	})
}

type paletteItem struct {
	label, hint, glyph string
	run                func()
	group              string
}

func (v *View) filterGoal(name string) {
	if v.goal == name {
		v.goal = ""
	} else {
		v.goal = name
	}
	v.layoutBoard()
}

func (v *View) confirmEnd(a *Agent) {
	title := a.Snapshot().Meta.Title
	v.showDialog(caution.Dialog("End "+title+"?").CardSize(440, 170).Kids(
		caution.Label("This stops its process. Its conversation stays, and the next message picks it up again.").
			FontSize(13).Color("$inkDim").Wrap().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("End session").Primary().OnClick(func() { v.closeDialog(); a.Stop() }),
		),
	))
}

func (v *View) confirmResume(list []*Agent) {
	var names []string
	for _, a := range list {
		names = append(names, "• "+a.Snapshot().Meta.Title)
	}
	if len(names) > 8 {
		names = append(names[:7], fmt.Sprintf("• and %d more", len(list)-7))
	}
	head := "Switchboard quit while this session was in the middle of a turn or had background work running:"
	if len(list) > 1 {
		head = fmt.Sprintf("Switchboard quit while these %d sessions were in the middle of a turn or had background work running:", len(list))
	}
	v.showDialog(caution.Dialog("Resume interrupted sessions?").CardSize(460, 170+18*float64(len(names))).Kids(
		caution.VStack().Gap(8).Align("stretch").Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}).Kids(
			caution.Label(head).FontSize(13).Color("$inkDim").Wrap(),
			caution.Label(strings.Join(names, "\n")).FontSize(13).Wrap(),
			caution.Label("Resume tells each one Switchboard restarted and to carry on where it left off.").FontSize(13).Color("$inkDim").Wrap(),
		),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Not now").OnClick(v.closeDialog),
			caution.Button("Resume all").Primary().OnClick(func() { v.closeDialog(); v.hub.Resume(list) }),
		),
	))
}

func (v *View) paletteItems() []paletteItem {
	items := []paletteItem{
		{"New session", modLabel + "N", icPlus, v.openNew, "Actions"},
		{"Ask the dispatcher", "", icSparkles, v.focusCommand, "Actions"},
		{"Write a message", modLabel + "L", icSend, v.focusInput, "Actions"},
		{"Next session that needs you", modLabel + "J", icHand, v.nextWaiting, "Actions"},
		{"All sessions", modLabel + "0", icLayers, v.closeFocus, "Actions"},
		{"Keyboard shortcuts", "?", icKeys, v.openShortcuts, "Actions"},
		{"Settings", modLabel + ",", icSettings, v.openSettings, "Actions"},
		{"Check for updates", "", icRetry, v.checkForUpdates, "Actions"},
	}
	if tag := v.hub.UpdateReady(); tag != "" {
		items = append(items, paletteItem{"Restart to update to " + strings.TrimPrefix(tag, "v"), "", icSettings, v.restartToUpdate, "Actions"})
	}
	if a := v.target(); a != nil {
		title := a.Snapshot().Meta.Title
		g := "On " + title
		items = append(items,
			paletteItem{"Allow what it asks", "A  " + modLabel + "Y", icCheck, func() { v.answer(true, false) }, g},
			paletteItem{"Deny what it asks", "D  " + shiftLabel + modLabel + "D", icFail, func() { v.answer(false, false) }, g},
			paletteItem{"Interrupt the turn", modLabel + ".", icPause, a.Interrupt, g},
			paletteItem{"Rename", "R", icCircle, func() { v.openRename(a) }, g},
			paletteItem{"End the session", "", icCircle, func() { v.confirmEnd(a) }, g},
			paletteItem{"Remove from the wall", modLabel + backspaceLabel, icCircle, func() { v.confirmRemove(a) }, g},
		)
	}
	for i, a := range v.shown {
		a := a
		s := a.Snapshot()
		hint := ""
		if i < 9 {
			hint = fmt.Sprintf("%s%d", modLabel, i+1)
		}
		items = append(items, paletteItem{s.Meta.Title + "  ·  " + providerName(s.Meta.Provider) + "  ·  " + s.State.String(),
			hint, stateIcon(s.State), func() { v.select_(a); v.openFocus(a) }, "Sessions"})
	}
	open, done := v.goalRows()
	for _, g := range open {
		name := g.name
		items = append(items, paletteItem{"Mark " + name + " done", "", icCheck, func() { v.markGoalDone(name) }, "Goals"})
	}
	for _, g := range done {
		name := g.name
		items = append(items, paletteItem{"Reopen " + name, "", icTarget, func() { go v.hub.SetGoalDone(name, false, true) }, "Goals"})
	}
	for _, g := range append(open, done...) {
		name := g.name
		items = append(items, paletteItem{"Remove " + name, "", icCircle, func() { v.confirmRemoveGoal(name) }, "Goals"})
	}
	if v.project != "" {
		items = append(items, paletteItem{"All projects", "", icFolder, func() { v.filterProject(v.project) }, "Projects"})
	}
	if names := v.projects(); len(names) > 1 {
		for _, name := range names {
			name := name
			items = append(items, paletteItem{"Show only " + name, "", icFolder, func() { v.filterProject(name) }, "Projects"})
		}
	}
	cur, _ := v.hub.ActiveUI()
	if cur != "" {
		items = append(items, paletteItem{"Use default interface", "", icLayers, func() { v.useUI("") }, "Interface"})
	}
	for _, name := range v.uiNames() {
		name := name
		if name != cur {
			items = append(items, paletteItem{"Use " + name + " interface", "", icLayers, func() { v.useUI(name) }, "Interface"})
		}
	}
	for _, name := range themeNames(v.set) {
		name := name
		items = append(items, paletteItem{name + " theme", "", icSettings, func() {
			go v.hub.UpdateSettings(func(s *Settings) { s.Theme = name })
		}, "Theme"})
	}
	return items
}

func (v *View) openPalette() {
	all := v.paletteItems()
	field := caution.TextField("").Placeholder("Type a command or a session, or a message for the dispatcher")
	list := caution.VStack().Gap(2).Align("stretch").NoFade().Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	var matches []paletteItem
	var rows []*caution.Node
	hi := 0
	run := func(it paletteItem) {
		v.closeDialog()
		it.run()
	}
	var render func()
	render = func() {
		list.Clear()
		rows = rows[:0]
		group := ""
		for i, it := range matches {
			it := it
			if it.group != group {
				group = it.group
				list.Add(caution.Panel().H(28).Kids(caution.Label(strings.ToUpper(group)).FontSize(11).Weight(650).Color("$inkFaint").
					Anchor(caution.A{Left: px(12), Bottom: px(4)})))
			}
			bg := "$panel"
			if i == hi {
				bg = "$control"
			}
			row := caution.Panel().H(36).Radius(6).Bg(bg).NoFade().Kids(
				caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { run(it) }),
				caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(12), Right: px(12), Top: px(0), Bottom: px(0)}).Kids(
					icon(it.glyph, 14, "$inkDim"),
					caution.Label(clip(it.label, 70)).FontSize(13).Fill(1),
				),
			)
			if it.hint != "" {
				row.Add(kbd(it.hint).Anchor(caution.A{Right: px(12), CenterY: px(0)}))
			}
			list.Add(row)
			rows = append(rows, row)
		}
		if len(matches) == 0 {
			list.Add(caution.Label("Enter sends this to the dispatcher").FontSize(13).Color("$inkFaint").Pad(10))
		}
	}
	filter := func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		matches = matches[:0]
		for _, it := range all {
			if q == "" || fuzzy(strings.ToLower(it.label), q) {
				matches = append(matches, it)
			}
		}
		hi = 0
		render()
	}
	field.OnInput(filter)
	field.OnNav(func(d int) {
		if len(matches) > 0 {
			rows[hi].Bg("$panel")
			hi = (hi + d + len(matches)) % len(matches)
			rows[hi].Bg("$control").Reveal()
		}
	})
	field.OnSubmit(func(q string) {
		if len(matches) > 0 {
			run(matches[hi])
			return
		}
		if strings.TrimSpace(q) != "" {
			v.closeDialog()
			go v.hub.Dispatcher.Send(q)
		}
	})
	filter("")
	d := caution.Dialog("").CardSize(620, 460).Kids(
		caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(16), Right: px(16), Top: px(14)}).Kids(
			icon(icSearch, 16, "$inkFaint"), field.Fill(1)),
		caution.Panel().H(1).Bg("$edgeSoft").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(60)}),
		caution.Scroll().Anchor(caution.A{Left: px(8), Right: px(8), Top: px(68), Bottom: px(8)}).Kids(list),
	)
	v.showDialog(d)
	field.Focus()
}

func fuzzy(s, q string) bool {
	i := 0
	for _, r := range s {
		if i < len(q) && byte(r) == q[i] {
			i++
		}
	}
	return i == len(q)
}

func (v *View) confirmRemove(a *Agent) {
	m := a.Snapshot().Meta
	text := "This ends its process and takes it off the wall. Its conversation stays on disk."
	buttons := []*caution.Node{
		caution.Button("Cancel").OnClick(v.closeDialog),
		caution.Button("Remove").Primary().OnClick(func() { v.closeDialog(); go v.hub.Remove(a, m.Worktree != "") }),
	}
	if m.Worktree != "" {
		left, err := worktreeLeftovers(m)
		switch {
		case err == nil && left == "":
			text += " Its worktree and branch " + m.WorkBranch + " have nothing unmerged, so they go too."
		default:
			if left == "" {
				left = "changes switchboard could not check"
			}
			text = "Its worktree has " + left + ". Keep worktree leaves it at " + shortPath(worktreeTop(m)) +
				" on branch " + m.WorkBranch + ". Delete worktree throws that work away."
			buttons = []*caution.Node{
				caution.Button("Cancel").OnClick(v.closeDialog),
				caution.Button("Delete worktree").OnClick(func() { v.closeDialog(); go v.hub.Remove(a, true) }),
				caution.Button("Keep worktree").Primary().OnClick(func() { v.closeDialog(); go v.hub.Remove(a, false) }),
			}
		}
	}
	d := caution.Dialog("Remove "+m.Title+"?").CardSize(480, 190).Kids(
		caution.Label(text).FontSize(13).Color("$inkDim").Wrap().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(buttons...),
	)
	v.showDialog(d)
}

func (v *View) buildEmpty() *caution.Node {
	agents := caution.VStack().Gap(6).Align("stretch").W(480)
	for _, p := range providers {
		ok := checkBin(v.hub, p) == nil
		status, color, glyph := "Installed", "$stReady", icCheck
		if !ok {
			status, color, glyph = "npm i -g "+map[string]string{"claude": "@anthropic-ai/claude-code", "codex": "@openai/codex", "gemini": "@google/gemini-cli"}[p], "$inkFaint", icCircle
		}
		agents.Add(caution.Panel().H(36).Bg("$panel").Radius(8).Border("$edgeSoft", 1).Kids(
			caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(12), Right: px(12), CenterY: px(0)}).Kids(
				icon(agentIcon(p), 14, "$inkDim"),
				caution.Label(providerName(p)).FontSize(13).Weight(560).Fill(1),
				icon(glyph, 13, color),
				caution.Label(status).FontSize(12).Color(color),
			)))
	}
	examples := caution.VStack().Gap(6).Align("stretch").W(480)
	for _, ex := range []string{
		"Start a Claude session in ~/code/app to fix the failing tests",
		"Split the settings page redesign across three Codex sessions",
		"Ask Gemini to write docs for the new billing API",
	} {
		ex := ex
		examples.Add(caution.Panel().H(40).Bg("$panel").Radius(8).Border("$edgeSoft", 1).Kids(
			caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { v.cmd.SetValueNow(ex); v.cmd.Focus() }),
			caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(14), Right: px(14), CenterY: px(0)}).Kids(
				icon(icSparkles, 14, "$accent"),
				caution.Label(ex).FontSize(13).Color("$inkDim"),
			)))
	}
	return caution.VStack().Gap(16).Align("center").Anchor(caution.A{CenterX: px(0), CenterY: px(-20)}).Kids(
		icon(icLayers, 40, "$inkFaint"),
		caution.VStack().Gap(6).Align("center").Kids(
			caution.Label("No sessions yet").FontSize(20).Weight(650),
			caution.Label("Start one yourself, or give the dispatcher a goal and it starts the sessions for you.").FontSize(13).Color("$inkDim"),
		),
		caution.HStack().Gap(8).Align("center").Kids(
			caution.Button("New session").Primary().OnClick(v.openNew), kbd(modLabel+"N"),
		),
		caution.Label("AGENTS").FontSize(11).Weight(650).Color("$inkFaint"),
		agents,
		caution.Label("TRY THE DISPATCHER").FontSize(11).Weight(650).Color("$inkFaint"),
		examples,
	)
}

func (v *View) openShortcuts() {
	rows := caution.Grid([]caution.GridCol{{Px: 120}, {Weight: 1, Align: "stretch"}}).ColGap(14).RowGap(10).
		Anchor(caution.A{Left: px(24), Right: px(24), Top: px(18)})
	for _, h := range [][2]string{
		{modLabel + "K", "Commands: every action and session"}, {modLabel + "N", "New session"}, {modLabel + "L", "Write in the open session, or ask the dispatcher"},
		{"J  K", "Move between sessions"}, {"Enter", "Open the selected session, write in the open one, or send the answers to a question"}, {modLabel + "1 … " + modLabel + "9", "Open a session by position"},
		{"A  " + modLabel + "Y", "Allow what the selected or first waiting session asks"}, {shiftLabel + "A  " + shiftLabel + modLabel + "Y", "Always allow it for that session"},
		{"D  " + shiftLabel + modLabel + "D", "Deny it"}, {modLabel + "J", "Jump to the next session that needs you"}, {".", "Interrupt the turn"},
		{modLabel + "E", "Edit the newest pending message"}, {shiftLabel + modLabel + "E", "Interrupt and send pending messages now"},
		{"R", "Rename"}, {modLabel + backspaceLabel, "Remove"}, {"Esc", "Leave the message box, then back to all sessions"}, {modLabel + ",", "Settings"},
	} {
		rows.Add(caution.HStack().Kids(kbd(h[0])))
		rows.Add(caution.Label(h[1]).FontSize(13).Color("$inkDim"))
	}
	v.showDialog(caution.Dialog("Keyboard shortcuts").CardSize(520, 590).Kids(rows))
}
