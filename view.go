package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	caution "github.com/nullentropy/caution"
)

var px = caution.Px

func fill() caution.A { return caution.A{Left: px(0), Right: px(0), Top: px(0), Bottom: px(0)} }

const (
	tileW    = 380.0
	tileH    = 138.0
	wallPad  = 24.0
	gap      = 14.0
	consoleH = 56.0
	statusW  = 420.0
)

type cueKind int

const (
	cueNeedsYou cueKind = iota
	cueDone
)

type View struct {
	s   *caution.Session
	hub *Hub

	root, body, wall, empty    *caution.Node
	area, field                *caution.Node
	boardStack, goals          *caution.Node
	sections                   []*section
	stats                      map[string]*caution.Node
	console                    *caution.Node
	dDot, dLine, dActions, cmd *caution.Node
	dOpen                      *caution.Node
	dStatus                    *caution.Node
	tiles                      map[*Agent]*tile
	order, shown               []*Agent
	sel                        *Agent
	native                     bool
	header, statsRow, compact  *caution.Node
	goal                       string
	showDone                   bool
	focus                      *focusView
	shell, holder, railList    *caution.Node
	project                    string
	projNames                  []string
	projBox, projSel           *caution.Node
	limitsBox                  *caution.Node
	updateBox                  *caution.Node
	rows                       map[*Agent]*railRow
	dialog                     *caution.Node
	askSend                    func()
	cols                       int
	set                        Settings
	dPend                      string
	dState0                    State
	lastPick                   time.Time
	lastPickX, lastPickY       float64
}

func mountView(hub *Hub, native bool) func(s *caution.Session) *caution.Node {
	return func(s *caution.Session) *caution.Node {
		v := &View{s: s, hub: hub, tiles: map[*Agent]*tile{}, set: hub.Settings(), dState0: -1, native: native}
		if native {
			setDockIcon(appIcon)
		}
		root := v.build()
		hub.attach(v)
		if list := hub.takeInterrupted(); len(list) > 0 {
			v.confirmResume(list)
		}
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-s.Done():
					hub.detach(v)
					return
				case <-t.C:
					s.Update(v.tick)
				}
			}
		}()
		return root
	}
}

func (v *View) stat(key, label string) *caution.Node {
	n := caution.Label("0").FontSize(22).Weight(650)
	v.stats[key] = n
	pick := func() {
		v.closeFocus()
		for _, s := range v.sections {
			if s.key == key && s.node.Parent() != nil {
				s.node.Reveal()
			}
		}
		if key == "need" {
			v.nextWaiting()
		}
	}
	return caution.Panel().W(88).H(46).Kids(
		caution.VStack().Gap(0).Anchor(caution.A{Left: px(0), CenterY: px(0)}).Kids(n, caution.Label(label).FontSize(11).Color("$inkFaint")),
		caution.Glass().Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { pick() }),
	)
}

func (v *View) menus() []caution.Menu {
	return []caution.Menu{
		{Title: "Switchboard", App: true, Items: []caution.MenuItem{
			{Title: "Check for Updates…", OnPick: v.checkForUpdates},
			{Title: "Settings…", Key: modKey + "+,", OnPick: v.openSettings},
		}},
		{Title: "File", Items: []caution.MenuItem{
			{Title: "New Session…", Key: modKey + "+n", OnPick: v.openNew},
		}},
		{Title: "Session", Items: []caution.MenuItem{
			{Title: "Write a Message", Key: modKey + "+l", OnPick: v.focusInput},
			{Title: "Next Session That Needs You", Key: modKey + "+j", OnPick: v.nextWaiting},
		}},
		{Title: "View", Items: []caution.MenuItem{
			{Title: "Commands…", Key: modKey + "+k", OnPick: v.openPalette},
			{Title: "All Sessions", Key: modKey + "+0", OnPick: v.closeFocus},
		}},
		{Title: "Help", Items: []caution.MenuItem{
			{Title: "Keyboard Shortcuts", OnPick: v.openShortcuts},
		}},
	}
}

func (v *View) build() *caution.Node {
	s := v.s
	s.SetTitle("Switchboard")
	s.SetTheme(themeTokens(v.set, v.set.Theme))
	s.SetMetrics(caution.MetricsCompact)
	s.PreloadSounds(soundNeed)
	s.AddFont("Inter", interURI)
	s.AddFont("JetBrains Mono", monoURI)
	s.AddFont("Lucide", iconURI)
	s.SetFonts("Inter", "JetBrains Mono")

	if runtime.GOOS != "linux" {
		s.SetMenu(v.menus()...)
	}
	v.bindKeys()

	v.stats = map[string]*caution.Node{}
	v.projBox = caution.HStack()
	inset := 0.0
	if v.native {
		inset = 72
	}
	v.limitsBox = caution.HStack().Gap(24).Align("center")
	v.updateBox = caution.HStack().Align("center")
	v.drawUpdate()
	v.drawLimits()
	v.compact = caution.RichText().FontSize(12).Color("$inkDim").Anchor(caution.A{Left: px(210 + inset), CenterY: px(0)})
	v.statsRow = caution.HStack().Gap(12).Align("center").Anchor(caution.A{Left: px(210 + inset), CenterY: px(0)}).Kids(
		v.stat("need", "Need you"), v.stat("work", "Working"), v.stat("done", "Done"), v.stat("spent", "Spent"),
	)
	v.header = caution.Panel().Dock("top").H(v.headerH(false)).Bg("$titlebar").WindowDrag().Kids(
		caution.HStack().Gap(9).Align("center").Anchor(caution.A{Left: px(20 + inset), CenterY: px(0)}).Kids(
			icon(icLayers, 18, "$accent"),
			caution.Label("Switchboard").FontSize(15).Weight(650),
		),
		v.statsRow,
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Right: px(16), CenterY: px(0)}).Kids(
			v.updateBox,
			v.limitsBox,
			v.projBox,
			caution.Button("New session  "+modLabel+"N").Primary().Tip("Or double-click empty space").OnClick(v.openNew),
		),
	)
	rule := caution.Panel().Dock("top").H(1).Bg("$edgeSoft")

	v.cols = colsFor(s.Viewport())
	v.wall = caution.Scroll().Anchor(fill()).Kids(v.buildBoard())
	v.empty = v.buildEmpty()

	floor := caution.Glass().Anchor(fill()).OnPick(func(x, y float64, _ *caution.Node) {
		now := time.Now()
		near := abs(x-v.lastPickX) < 8 && abs(y-v.lastPickY) < 8
		if near && now.Sub(v.lastPick) < 450*time.Millisecond {
			v.lastPick = time.Time{}
			v.openNew()
			return
		}
		v.lastPick, v.lastPickX, v.lastPickY = now, x, y
	})
	v.field = caution.Panel().Kids(floor, v.wall)

	v.console = v.buildConsole()
	v.area = caution.DockPanel().Anchor(fill()).Kids(v.console, v.field)
	v.body = caution.Panel().Bg("$bg").Kids(v.area)

	for _, a := range v.hub.Agents() {
		v.addTile(a)
	}
	v.layoutBoard()
	v.syncEmpty()
	v.refreshSummary()
	v.updateDispatcher(v.hub.Dispatcher.Snapshot())

	s.OnResize(func(w, h float64) {
		v.fitConsole(w, h)
		if c := colsFor(w, h); c != v.cols {
			v.cols = c
			for _, sec := range v.sections {
				switch {
				case sec.inbox:
				case miniGroup(sec.key):
					sec.body.SetProp("columns", caution.Grid(gridCols(c+1)).Prop("columns"))
				default:
					sec.body.SetProp("columns", caution.Grid(gridCols(c)).Prop("columns"))
				}
			}
		}
	})

	v.root = caution.DockPanel().Kids(v.header, rule, v.body)
	return v.root
}

func (v *View) headerH(compact bool) float64 {
	switch {
	case v.native:
		return 66
	case compact:
		return 44
	}
	return 60
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:max(n-1, 1)]) + "…"
	}
	return s
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (v *View) buildConsole() *caution.Node {
	d := v.hub.Dispatcher
	v.dDot = caution.Panel().W(8).H(8).Radius(4)
	v.dLine = caution.Label("").FontSize(12).Color("$inkDim").Anchor(caution.A{Left: px(0), Right: px(0), CenterY: px(0)})
	v.dActions = caution.HStack().Gap(6).Align("center")
	v.dOpen = caution.HStack().Align("center")
	v.dStatus = caution.HStack().Gap(8).Align("center").W(statusW).Clips().
		Kids(v.dDot, caution.Panel().Fill(1).H(consoleH).Clips().Kids(v.dLine), v.dOpen, v.dActions)
	if v.s != nil {
		v.fitConsole(v.s.Viewport())
	}
	v.cmd = caution.TextField("").Placeholder("Ask the dispatcher to start, steer or report on sessions, or to run a goal as a swarm")
	v.cmd.OnSubmit(func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		v.cmd.ClearValue()
		go d.Send(text)
	})
	return caution.Panel().Dock("bottom").H(consoleH).Bg("$titlebar").Clips().Kids(
		caution.Panel().H(1).Bg("$edgeSoft").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)}),
		caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(16), Right: px(16), CenterY: px(0)}).Kids(
			icon(icSparkles, 16, "$accent"), v.cmd.Fill(1), kbd(modLabel+"L"), caution.Panel().W(8),
			caution.Button(shortcutsLabel).Tip("Every keyboard shortcut").OnClick(v.openShortcuts), v.dStatus),
	)
}

const shortcutsLabel = "Shortcuts  ?"

func (v *View) fitConsole(w, _ float64) {
	help := 16 + 16 + 10 + 10 + textW(modLabel+"L", 10, true) + 10 + 10 + 8 + 10 + textW(shortcutsLabel, 13, false) + 2*16 + 10 + 16
	v.dStatus.W(min(statusW, max(0, w-help)))
}

func (v *View) focusInput() {
	if v.focus == nil {
		v.focusCommand()
		return
	}
	v.closeDialog()
	v.focus.composer.Focus()
}

func (v *View) focusCommand() {
	if v.focus != nil && v.focus.a == v.hub.Dispatcher {
		v.closeDialog()
		v.focus.composer.Focus()
		return
	}
	v.closeFocus()
	v.closeDialog()
	v.cmd.Focus()
}

func (v *View) updateDispatcher(s Snapshot) {
	tok := "$" + stateToken(s.State)
	v.dDot.Bg(tok)
	line, color := "Dispatcher is idle", "$inkFaint"
	widget := false
	for i := len(s.Entries) - 1; i >= 0; i-- {
		e := s.Entries[i]
		if e.Kind == EntryAssistant {
			line, color = plain(e.Text), "$inkDim"
			break
		}
		if e.Kind == EntryWidget {
			line, color, widget = "Dispatcher shows a "+e.Widget.Kind, "$inkDim", true
			if e.Widget.Title != "" {
				line += ": " + e.Widget.Title
			}
			break
		}
		if e.Kind == EntryUser {
			break
		}
	}
	if s.State == Working || s.State == Starting {
		line, color = "Dispatcher: "+strings.TrimPrefix(s.Activity, "mcp__switchboard__"), "$inkDim"
	}
	if len(s.Pending) > 0 {
		p := s.Pending[0]
		line, color = strings.TrimSpace("Dispatcher wants "+p.Tool+" "+p.Summary), "$stNeed"
	}
	if n := len(s.Queued); n > 0 {
		line = fmt.Sprintf("%d pending  ·  %s", n, line)
	}
	v.dLine.SetText(strings.Join(strings.Fields(line), " ")).SetColor(color).Tip(line)
	if widget != (len(v.dOpen.Children()) > 0) {
		v.dOpen.Clear()
		if widget {
			v.dOpen.Add(caution.Button("Open").Tip("Show the dispatcher's conversation").
				OnClick(func() { v.openFocus(v.hub.Dispatcher) }))
		}
	}

	key := ""
	if len(s.Pending) > 0 {
		key = s.Pending[0].ReqID
	}
	if key != v.dPend || s.State != v.dState0 {
		v.dPend, v.dState0 = key, s.State
		v.dActions.Clear()
		d := v.hub.Dispatcher
		if key != "" {
			p := s.Pending[0]
			v.dActions.Kids(
				caution.Button("Deny").OnClick(func() { d.Answer(p.ReqID, false, false, nil) }),
				caution.Button("Allow").Primary().OnClick(func() { d.Answer(p.ReqID, true, false, nil) }),
			)
		}
		if s.State == Working || s.State == Starting {
			v.dActions.Add(caution.Button("Interrupt").Tip("Interrupt the dispatcher").OnClick(d.Interrupt))
		}
		v.dActions.Add(caution.Button("Conversation").Tip("The dispatcher's whole conversation").OnClick(func() { v.openFocus(d) }))
	}
}

func (v *View) play(src string) {
	if v.set.Sounds {
		v.s.Play(src)
	}
}

func colsFor(w, _ float64) int {
	c := int((w - 2*wallPad + gap) / (tileW + gap))
	return max(1, min(c, 6))
}

func gridCols(n int) []caution.GridCol {
	cols := make([]caution.GridCol, n)
	for i := range cols {
		cols[i] = caution.GridCol{Weight: 1, Align: "stretch"}
	}
	return cols
}

func (v *View) syncEmpty() {
	if len(v.order) == 0 && v.empty.Parent() == nil {
		v.wall.Remove()
		v.field.Add(v.empty)
	} else if len(v.order) > 0 && v.empty.Parent() != nil {
		v.empty.Remove()
		v.field.Add(v.wall)
	}
}

func (v *View) addTile(a *Agent) {
	t := newTile(v, a)
	v.tiles[a] = t
	v.order = append(v.order, a)
	t.update(a.Snapshot())
}

func (v *View) agentAdded(a *Agent) {
	v.s.Update(func() {
		if v.tiles[a] == nil {
			v.addTile(a)
			v.layoutBoard()
			v.syncEmpty()
			v.refreshSummary()
		}
	})
}

func (v *View) agentRemoved(a *Agent) {
	v.s.Update(func() {
		if v.removeTile(a) {
			v.syncEmpty()
			v.refreshSummary()
		}
	})
}

func (v *View) removeTile(a *Agent) bool {
	if v.tiles[a] == nil {
		return false
	}
	var next *Agent
	if v.focus != nil && v.focus.a == a {
		list, _ := v.railOrder()
		next = afterRemoval(v.hub.Dispatcher, list, a)
	}
	delete(v.tiles, a)
	for i, x := range v.order {
		if x == a {
			v.order = append(v.order[:i:i], v.order[i+1:]...)
			break
		}
	}
	if v.sel == a {
		v.sel = nil
	}
	v.layoutBoard()
	if next != nil {
		v.switchTo(next)
	}
	return true
}

func (v *View) agentChanged(a *Agent) {
	snap := a.Snapshot()
	v.s.Update(func() {
		if a == v.hub.Dispatcher {
			v.updateDispatcher(snap)
		}
		v.updateTile(a, snap)
		if v.focus != nil {
			v.updateRow(a, snap)
			if v.focus.a == a {
				v.focus.update(snap)
			}
		}
		v.refreshSummary()
	})
}

func (v *View) updateTile(a *Agent, snap Snapshot) {
	t := v.tiles[a]
	if t == nil {
		return
	}
	was, card := groupOf(t.state), t.card
	state, title, goal := t.state, t.snap.Meta.Title, t.snap.Meta.Goal
	t.update(snap)
	if groupOf(t.state) != was || t.card != card ||
		(goal != "" || snap.Meta.Goal != "") && (t.state != state || snap.Meta.Title != title || snap.Meta.Goal != goal) {
		v.layoutBoard()
		if v.sel == a {
			t.setSelected(true)
		}
	}
}

func (v *View) cue(_ *Agent, k cueKind) {
	if k != cueNeedsYou {
		return
	}
	v.s.Update(func() { v.play(soundNeed) })
}

func (v *View) settingsChanged() {
	set := v.hub.Settings()
	v.s.Update(func() {
		themeChanged := set.Theme != v.set.Theme || !maps.Equal(themeTokens(set, set.Theme), themeTokens(v.set, set.Theme))
		v.set = set
		v.drawUpdate()
		if themeChanged {
			v.s.SetTheme(themeTokens(set, set.Theme))
			for _, t := range v.tiles {
				t.restyle()
				t.statusFX.retint(set)
			}
			for _, r := range v.rows {
				r.lineFX.retint(set)
			}
			if v.focus != nil {
				v.focus.restyle()
				v.focus.activityFX.retint(set)
			}
		}
	})
}

func (v *View) refreshSummary() {
	count := map[string]int{}
	cost := 0.0
	for _, a := range v.order {
		if t := v.tiles[a]; t != nil {
			count[groupOf(t.state)]++
			cost += t.cost
		}
	}
	for _, k := range []string{"need", "work", "done"} {
		c := "$ink"
		if count[k] == 0 {
			c = "$inkFaint"
		} else if k == "need" {
			c = "$stNeed"
		}
		v.stats[k].SetText(fmt.Sprint(count[k])).SetColor(c)
	}
	v.stats["spent"].SetText(fmt.Sprintf("$%.2f", cost)).SetColor(map[bool]string{true: "$ink", false: "$inkFaint"}[cost > 0])
	var spans []caution.Span
	for _, c := range []struct{ k, label, tok string }{{"need", "need you", "stNeed"}, {"work", "working", "stWork"}, {"done", "done", "stReady"}} {
		if count[c.k] > 0 {
			spans = append(spans, caution.Span{Text: "● ", Color: "$" + c.tok}, caution.Span{Text: fmt.Sprintf("%d %s     ", count[c.k], c.label)})
		}
	}
	spans = append(spans, caution.Span{Text: fmt.Sprintf("$%.2f", cost), Color: "$inkFaint"})
	v.compact.SetSpans(spans...)
	title := "Switchboard"
	if n := count["need"]; n > 0 {
		title = fmt.Sprintf("Switchboard (%d need you)", n)
	}
	v.s.SetTitle(title)
	v.syncProjects()
}

func (v *View) tick() {
	for _, t := range v.tiles {
		t.tickClock()
	}
	if v.focus != nil {
		v.focus.tickClock()
	}
}

func (v *View) closeDialog() {
	v.askSend = nil
	if v.dialog != nil {
		v.dialog.Remove()
		v.dialog = nil
	}
}

func (v *View) showDialog(d *caution.Node) {
	v.closeDialog()
	v.dialog = d.OnDismiss(v.closeDialog)
	v.body.Add(d)
}

var (
	modes      = []string{"default", "acceptEdits", "auto", "plan", "bypassPermissions"}
	modeLabels = []string{"Ask first", "Accept edits", "Auto", "Plan", "Bypass permissions"}
)

func indexOf(xs []string, x string) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return 0
}

func (v *View) openNew() {
	set := v.hub.Settings()
	here := ""
	if v.focus != nil {
		here = sessionFolder(v.focus.a.Snapshot().Meta)
	}
	recent := v.hub.recentFolders(here)
	cwd := ""
	if len(recent) > 0 {
		cwd = recent[0]
	} else {
		cwd, _ = os.Getwd()
	}
	model, mode, provider := set.Model, set.Mode, set.Provider
	if mode == "" {
		mode = "default"
	}
	folder := caution.TextField(cwd).Placeholder("/path/to/repo")
	prompt := caution.Textarea("").Placeholder("What should it do? The first line names the session (optional)").Rows(5).EnterSends()
	errL := caution.Label("").FontSize(12).Color("$stFail")
	for _, f := range []*caution.Node{folder, prompt} {
		f.OnInput(func(string) {})
	}
	labels := make([]string, len(providers))
	for i, p := range providers {
		labels[i] = providerName(p)
	}
	modelBox := caution.Panel().H(32)
	pickModel := func() {
		names := append([]string{"Default"}, v.hub.models(provider)...)
		if model != "" && indexOf(names, model) == 0 && names[0] != model {
			names = append(names, model)
		}
		sel := caution.Select(names, indexOf(names, model)).OnSelect(func(i int) {
			model = ""
			if i > 0 {
				model = names[i]
			}
		})
		modelBox.Clear()
		modelBox.Add(sel.Anchor(fill()))
	}
	pickModel()
	provSel := caution.Select(labels, indexOf(providers, provider)).OnSelect(func(i int) {
		if providers[i] != provider {
			model = ""
		}
		provider = providers[i]
		pickModel()
	})
	modeSel := caution.Select(modeLabels, indexOf(modes, mode)).OnSelect(func(i int) { mode = modes[i] })

	goalF := caution.TextField("").Placeholder("Optional: groups sessions working toward one thing")
	goalF.OnInput(func(string) {})
	wt := v.hub.RepoBusy(expandHome(cwd))
	wtBox := caution.Checkbox("Separate worktree on a new branch", wt).OnToggle(func(b bool) { wt = b })
	pickFolder := func(s string) {
		wt = v.hub.RepoBusy(expandHome(strings.TrimSpace(s)))
		wtBox.SetChecked(wt)
	}
	folder.OnInput(pickFolder)
	folderBox := caution.Panel().H(32)
	if len(recent) > 0 {
		names := make([]string, len(recent), len(recent)+1)
		for i, p := range recent {
			names[i] = shortPath(p)
		}
		names = append(names, "Other…")
		folderBox.Add(caution.Select(names, 0).OnSelect(func(i int) {
			if i == len(recent) {
				folderBox.Clear()
				folderBox.Add(folder.Anchor(fill()))
				folder.Focus()
				return
			}
			folder.SetValue(recent[i])
			pickFolder(recent[i])
		}).Anchor(fill()))
	} else {
		folderBox.Add(folder.Anchor(fill()))
	}
	start := func() {
		dir := expandHome(strings.TrimSpace(valueOf(folder)))
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			errL.SetText("That folder does not exist.")
			return
		}
		if err := checkBin(v.hub, provider); err != nil {
			errL.SetText(err.Error())
			return
		}
		v.hub.UpdateSettings(func(s *Settings) { s.Model, s.Mode, s.Provider = model, mode, provider })
		a, err := v.hub.New(Meta{Provider: provider, Title: titleFrom(valueOf(prompt), dir), Cwd: dir, Model: model, Mode: mode, Goal: strings.TrimSpace(valueOf(goalF))}, wt)
		if err != nil {
			errL.SetText(err.Error())
			return
		}
		v.closeDialog()
		if p := strings.TrimSpace(valueOf(prompt)); p != "" {
			go a.Send(p)
		} else {
			v.openFocus(a)
		}
	}
	for _, f := range []*caution.Node{prompt, folder, goalF} {
		f.OnSubmit(func(string) { start() })
	}

	lbl := func(s string) *caution.Node { return caution.Label(s).FontSize(13).Color("$inkDim") }
	form := caution.Grid([]caution.GridCol{{Px: 90}, {Weight: 1, Align: "stretch"}}).ColGap(12).RowGap(10).
		Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}).Kids(
		lbl("Folder"), folderBox,
		lbl("Agent"), provSel,
		lbl("Model"), modelBox,
		lbl("Permissions"), modeSel,
		lbl("Goal"), goalF,
		lbl("Worktree"), wtBox,
		lbl("Prompt"), prompt,
	)
	d := caution.Dialog("New session").CardSize(560, 470).Kids(
		form,
		errL.Anchor(caution.A{Left: px(20), Bottom: px(26)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Start").Primary().Tip("Enter from any field. Shift+Enter adds a line to the prompt").OnClick(start),
		),
	)
	v.showDialog(d)
	prompt.Focus()
}

func (v *View) openRename(a *Agent) {
	field := caution.TextField(a.Snapshot().Meta.Title)
	save := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			v.hub.Rename(a, s)
		}
		v.closeDialog()
	}
	field.OnSubmit(save)
	d := caution.Dialog("Rename session").CardSize(420, 150).Kids(
		field.Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Rename").Primary().OnClick(func() { save(valueOf(field)) }),
		),
	)
	field.OnInput(func(string) {})
	v.showDialog(d)
	field.Focus()
}

const spriteCheckH = 24

func (v *View) spriteChecks(set Settings) (*caution.Node, float64) {
	names, on := spriteNames(set), spritesOn(set)
	var checks []*caution.Node
	for _, name := range names {
		var cb *caution.Node
		cb = caution.Checkbox(name, slices.Contains(on, name)).H(spriteCheckH).OnToggle(func(b bool) {
			kept := true
			v.hub.UpdateSettings(func(s *Settings) { kept = setSpriteOn(s, name, b) })
			if !kept {
				cb.SetChecked(true)
			}
		})
		checks = append(checks, cb)
	}
	h := float64(min((len(names)+1)/2, 5) * spriteCheckH)
	grid := caution.Grid([]caution.GridCol{{Weight: 1}, {Weight: 1}}).ColGap(8).
		Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)}).Kids(checks...)
	return caution.Scroll().H(h).Kids(grid), h
}

func (v *View) openSettings() {
	set := v.hub.Settings()
	names := themeNames(set)
	theme := caution.Select(names, indexOf(names, set.Theme)).OnSelect(func(i int) {
		v.hub.UpdateSettings(func(s *Settings) { s.Theme = names[i] })
	})
	snd := caution.Checkbox("A soft tone when a session needs you", set.Sounds).OnToggle(func(b bool) {
		v.hub.UpdateSettings(func(s *Settings) { s.Sounds = b })
	})
	models, cur := v.hub.dispatcherModelChoices()
	dmodel := caution.Select(models, cur).OnSelect(func(i int) {
		model := ""
		if i > 0 {
			model = models[i]
		}
		v.hub.SetDispatcherModel(model)
	})
	upd := caution.Checkbox("Update automatically", !set.UpdatesOff).OnToggle(func(b bool) {
		v.hub.UpdateSettings(func(s *Settings) { s.UpdatesOff = !b })
	})
	lbl := func(s string) *caution.Node { return caution.Label(s).FontSize(13).Color("$inkDim") }
	spr, sprH := v.spriteChecks(set)
	d := caution.Dialog("Settings").CardSize(480, 288+sprH).Kids(
		caution.Grid([]caution.GridCol{{Px: 120}, {Weight: 1, Align: "stretch"}}).ColGap(12).RowGap(12).
			Anchor(caution.A{Left: px(20), Right: px(20), Top: px(18)}).Kids(
			lbl("Theme"), theme,
			lbl("Sound"), snd,
			lbl("Dispatcher model"), dmodel,
			lbl("Updates"), upd,
			lbl("Sprites"), spr,
		),
		caution.Button("Done").Primary().Anchor(caution.A{Right: px(20), Bottom: px(20)}).OnClick(v.closeDialog),
	)
	v.showDialog(d)
}

func valueOf(n *caution.Node) string {
	s, _ := n.Prop("value").(string)
	return s
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = home + p[1:]
	}
	return filepath.Clean(p)
}

func (v *View) updateChanged() { v.s.Update(v.drawUpdate) }

func (v *View) drawUpdate() {
	v.updateBox.Clear()
	if tag := v.hub.UpdateReady(); tag != "" {
		v.updateBox.Add(caution.Button("Update " + strings.TrimPrefix(tag, "v") + " ready").
			Tip("Restart Switchboard into the new version").OnClick(v.restartToUpdate))
	}
}

func (v *View) checkForUpdates() {
	v.updateDialog("Checking for updates…", "")
	wait := v.dialog
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		tag, ready, err := v.hub.CheckForUpdates(ctx)
		cancel()
		v.s.Update(func() {
			if v.dialog != wait {
				return
			}
			have := "You have version " + strings.TrimPrefix(version, "v") + "."
			next := strings.TrimPrefix(tag, "v")
			switch {
			case err != nil:
				v.updateDialog("Could not check for updates", sentence(err))
			case tag == "":
				v.updateDialog("Switchboard is up to date", have)
			case ready:
				v.updateDialog("Switchboard "+next+" is ready", have+" Restart Switchboard to update.",
					caution.Button("Later").OnClick(v.closeDialog),
					caution.Button("Restart").Primary().OnClick(func() { v.confirmRestart(tag) }))
			default:
				v.updateDialog("Switchboard "+next+" is available", have+" Automatic updates are off.",
					caution.Button("Later").OnClick(v.closeDialog),
					caution.Button("Update").Primary().OnClick(v.downloadUpdate))
			}
		})
	}()
}

func (v *View) downloadUpdate() {
	v.updateDialog("Downloading the update…", "")
	wait := v.dialog
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		tag, err := v.hub.DownloadUpdate(ctx)
		cancel()
		v.s.Update(func() {
			if v.dialog != wait {
				return
			}
			if err != nil {
				v.updateDialog("Could not download the update", sentence(err))
				return
			}
			v.confirmRestart(tag)
		})
	}()
}

func sentence(err error) string {
	s := err.Error()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func (v *View) updateDialog(title, body string, buttons ...*caution.Node) {
	if len(buttons) == 0 && body != "" {
		buttons = []*caution.Node{caution.Button("OK").Primary().OnClick(v.closeDialog)}
	}
	h := 110.0
	if body != "" {
		h = 170
	}
	d := caution.Dialog(title).CardSize(460, h)
	if body != "" {
		d.Add(caution.Label(body).FontSize(13).Color("$inkDim").Wrap().
			Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}))
	}
	if len(buttons) > 0 {
		d.Add(caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(buttons...))
	}
	v.showDialog(d)
}

func (v *View) restartToUpdate() { v.confirmRestart(v.hub.UpdateReady()) }

func (v *View) confirmRestart(tag string) {
	if tag == "" {
		return
	}
	names := v.hub.midTurn()
	if len(names) == 0 {
		v.closeDialog()
		go v.restartNow()
		return
	}
	head := "Restarting stops the turn these sessions are in. You can resume them after."
	v.showDialog(caution.Dialog("Restart to update to "+strings.TrimPrefix(tag, "v")+"?").CardSize(460, 150+18*float64(len(names))).Kids(
		caution.VStack().Gap(8).Align("stretch").Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}).Kids(
			caution.Label(head).FontSize(13).Color("$inkDim").Wrap(),
			caution.Label(strings.Join(names, "\n")).FontSize(13).Wrap(),
		),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Restart").Primary().OnClick(func() { v.closeDialog(); go v.restartNow() }),
		),
	))
}

func (v *View) restartNow() {
	err := v.hub.RestartToUpdate()
	if err == nil {
		return
	}
	v.s.Update(func() {
		v.showDialog(caution.Dialog("Could not update").CardSize(460, 160).Kids(
			caution.Label(err.Error()+". Switchboard is still running the current version.").FontSize(13).Color("$inkDim").Wrap().
				Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
			caution.Button("OK").Primary().Anchor(caution.A{Right: px(20), Bottom: px(20)}).OnClick(v.closeDialog),
		))
	})
}
