package main

import (
	"fmt"
	"strings"
	"time"

	caution "github.com/nullentropy/caution"
)

type shown struct {
	rev    int
	gutter *caution.Node
	body   *caution.Node
	mark   *stateMark
	timer  *caution.Node
	start  time.Time
	end    time.Time
	detail *caution.Node
	open   bool
	run    *toolRun
}

type focusView struct {
	v *View
	a *Agent

	node, title, stateL, sub, list, scroll, card, activity, clock, statusBar, bar, composer *caution.Node
	note, noteBar                                                                           *caution.Node
	icon                                                                                    *stateMark
	modeSel                                                                                 *caution.Node

	shown      []shown
	run        *toolRun
	state      State
	pendKey    string
	asked      string
	turnFrom   time.Time
	cost       float64
	asks       []userRow
	mention    *mentions
	ctxUsed    int
	ctxWindow  int
	activityFX glint
	cleared    int
}

type userRow struct {
	row  *caution.Node
	text string
	done bool
}

func (v *View) openFocus(a *Agent) {
	if v.showFocus(a) && a.Snapshot().State != NeedsYou {
		v.focus.composer.Focus()
	}
}

func (v *View) showFocus(a *Agent) bool {
	if v.focus != nil {
		if v.focus.a == a {
			return false
		}
		v.focus.node.Remove()
		v.focus = nil
	}
	v.closeDialog()
	v.setCompact(true)
	f := &focusView{v: v, a: a, state: -1}
	f.build()
	v.focus = f
	if v.shell == nil {
		v.buildShell()
	}
	v.holder.Add(f.node)
	if v.shell.Parent() == nil {
		v.body.Add(v.shell)
	}
	v.layoutRail()
	f.update(a.Snapshot())
	return true
}

func (v *View) closeFocus() {
	if v.focus == nil {
		return
	}
	v.closeDialog()
	v.focus.node.Remove()
	v.shell.Remove()
	v.focus = nil
	v.setCompact(false)
}

func (v *View) setCompact(on bool) {
	if on && v.compact.Parent() == nil {
		v.statsRow.Remove()
		v.header.Add(v.compact)
		v.header.H(v.headerH(true))
	} else if !on && v.statsRow.Parent() == nil {
		v.compact.Remove()
		v.header.Add(v.statsRow)
		v.header.H(v.headerH(false))
	}
}

func (f *focusView) build() {
	a := f.a
	snap := a.Snapshot()
	f.title = caution.Label("").FontSize(15).Weight(650)
	f.stateL = caution.Label("").FontSize(12).Weight(550)
	f.sub = caution.Label("").FontSize(12).Color("$inkFaint")
	f.modeSel = caution.Select(modeLabels, indexOf(modes, snap.Meta.Mode)).Tip("Permission mode").
		OnSelect(func(i int) { go a.SetMode(modes[i]) })

	f.icon = newMark(stateIcon(snap.State), 16, "$stWork")
	agentLine := providerName(snap.Meta.Provider)
	if snap.Meta.Model != "" {
		agentLine += " · " + snap.Meta.Model
	}
	top := caution.Panel().Dock("top").H(56).Bg("$titlebar").Kids(
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(14), CenterY: px(0)}).Kids(
			iconButton(icBack, "All sessions (Esc)", f.v.closeFocus)),
		caution.VStack().Gap(3).Anchor(caution.A{Left: px(58), Right: px(320), CenterY: px(0)}).Kids(
			caution.HStack().Gap(9).Align("center").Kids(f.icon.box, f.title, chip(agentLine, agentIcon(snap.Meta.Provider)), f.stateL),
			f.sub,
		),
		caution.HStack().Gap(8).Align("center").Anchor(caution.A{Right: px(14), CenterY: px(0)}).Kids(
			f.modeSel.W(150),
			iconButton(icPause, "Interrupt the turn ("+modLabel+".)", a.Interrupt),
			iconButton(icPower, "End session…", func() { f.v.confirmEnd(a) }),
		),
	)

	f.list = caution.VStack().Gap(16).Pad(28).Align("stretch").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	f.scroll = caution.Scroll().StickBottom().NoFade().Kids(f.list)

	f.card = caution.Panel().Dock("bottom").H(0).Bg("$needBg").Clips()

	f.activity = caution.Label("").FontSize(12).Mono().Color("$inkDim").
		Anchor(caution.A{Left: px(0), Right: px(0), CenterY: px(0)})
	f.activityFX.n = f.activity
	f.clock = caution.Label("").FontSize(12).Color("$inkFaint")
	f.statusBar = caution.Panel().Dock("bottom").H(30).Bg("$panelAlt").Clips().Kids(
		caution.HStack().Gap(20).Align("center").Anchor(caution.A{Left: px(24), Right: px(24), CenterY: px(0)}).Kids(
			caution.Panel().Fill(1).H(30).Clips().Kids(f.activity),
			hints([][2]string{{"Esc", "leave the message box"}, {"J K", "switch"}, {"Esc Esc", "all sessions"}}), f.clock))

	f.composer = caution.Textarea("").Placeholder(f.hint()).Rows(2).EnterSends().
		Anchor(caution.A{Left: px(20), Right: px(120), Top: px(14), Bottom: px(14)})
	f.mention = newMentions(f)
	f.composer.OnInput(func(s string) {
		f.say("")
		f.mention.input(s)
	})
	f.composer.OnTab(f.mention.tab)
	f.composer.OnSubmit(f.submit)
	f.note = caution.Label("").FontSize(12).Color("$stNeed").Anchor(caution.A{Left: px(24), Right: px(24), CenterY: px(0)})
	f.noteBar = caution.Panel().Dock("bottom").H(0).Bg("$panel").Clips().Kids(f.note)
	compose := caution.Panel().Dock("bottom").H(84).Bg("$panel").Kids(
		f.composer,
		caution.Button("Send").Primary().W(84).Anchor(caution.A{Right: px(20), CenterY: px(0)}).
			OnClick(func() { f.send(valueOf(f.composer)) }),
	)

	dock := caution.DockPanel().Anchor(fill()).Kids(
		top,
		caution.Panel().Dock("top").H(1).Bg("$edgeSoft"),
		compose,
		f.mention.panel,
		f.noteBar,
		caution.Panel().Dock("bottom").H(1).Bg("$edgeSoft"),
		f.statusBar,
		f.card,
		f.scroll,
	)
	f.node = caution.Panel().Bg("$bg").Anchor(fill()).Kids(dock)
}

func (f *focusView) submit(s string) {
	if f.mention.tab(false, s, -1) {
		return
	}
	f.send(s)
}

func (f *focusView) send(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	f.composer.ClearValue()
	f.mention.input("")
	if w := strings.Fields(text); w[0] == "/clear" && f.a != f.v.hub.Dispatcher {
		f.say("/clear is only available in the dispatcher. Clearing a session would drop the context it needs to finish its task.")
		return
	}
	f.say("")
	go f.a.Send(text)
}

func (f *focusView) say(text string) {
	f.note.SetText(text)
	f.noteBar.H(map[bool]float64{true: 0, false: 28}[text == ""])
}

func (f *focusView) update(s Snapshot) {
	tok := "$" + stateToken(s.State)
	f.title.SetText(s.Meta.Title)
	f.stateL.SetText(s.State.String()).SetColor(tok)
	f.icon.set(stateIcon(s.State), tok)
	where := shortPath(s.Meta.Cwd)
	if s.Branch != "" {
		where += "  ·  " + s.Branch
	}
	if s.Meta.Goal != "" {
		where = s.Meta.Goal + "  ·  " + where
	}
	f.sub.SetText(where)
	f.turnFrom, f.cost = s.TurnFrom, s.Cost
	f.ctxUsed, f.ctxWindow = s.CtxUsed, s.CtxWindow

	act := s.Activity
	switch {
	case s.State == Failed:
		act = oneLine(s.Err)
	case act == "" && s.State == Ready:
		act = "Ready"
	case s.State == Interrupted:
		act = "Interrupted when Switchboard quit mid-turn. Sending a message picks it up again."
	case act == "" && s.State == Stopped:
		act = "Ended. Sending a message picks the conversation up again."
	case s.State == NeedsYou:
		act = "Waiting for you"
	}
	f.activity.SetText(act)
	f.activityFX.set(f.v.set, act, 12, true, thinking(s))
	f.activity.SetColor(map[bool]string{true: tok, false: "$inkDim"}[s.State == NeedsYou || s.State == Failed || s.State == Interrupted])

	if s.Cleared != f.cleared {
		f.cleared = s.Cleared
		f.list.Clear()
		f.shown, f.run = nil, nil
	}
	for i, e := range s.Entries {
		if i >= len(f.shown) {
			f.shown = append(f.shown, f.addEntry(e))
			continue
		}
		if f.shown[i].rev != e.Rev {
			f.refreshEntry(&f.shown[i], e)
		}
	}
	for n := range f.asks {
		u := &f.asks[n]
		if u.done || n >= len(s.Meta.Checkpoints) {
			continue
		}
		u.done = true
		if s.Meta.Checkpoints[n] == "" {
			continue
		}
		n, text := n, u.text
		u.row.Add(caution.Panel().W(28).H(20).Kids(
			icon(icRetry, 13, "$inkFaint").Anchor(caution.A{CenterX: px(0), CenterY: px(0)}),
			caution.Glass().HoverFill("$hover").Anchor(fill()).Tip("Restore files to before this message").
				OnPick(func(_, _ float64, _ *caution.Node) { f.confirmRestore(n, text) }),
		))
	}

	key := ""
	if len(s.Pending) > 0 {
		key = s.Pending[0].ReqID
	}
	if s.State != f.state || key != f.pendKey {
		f.state = s.State
		f.pendKey = key
		f.buildCard(s)
		f.restyle()
		if s.State == NeedsYou {
			f.statusBar.H(0)
		} else {
			f.statusBar.H(30)
		}
	}
	f.tickClock()
}

func (sh *shown) tickTimer() {
	if sh.timer == nil || sh.start.IsZero() {
		return
	}
	switch {
	case sh.end.IsZero():
		sh.timer.SetText(elapsed(time.Since(sh.start)))
	case sh.end.Sub(sh.start) >= 3*time.Second:
		sh.timer.SetText(elapsed(sh.end.Sub(sh.start)))
	default:
		sh.timer.SetText("")
	}
}

func toolName(t string) string {
	if strings.HasPrefix(t, "mcp__") {
		if i := strings.LastIndex(t, "__"); i > 4 {
			t = t[i+2:]
		}
	}
	if textW(t, 12, false) <= 58 {
		return t
	}
	r := []rune(t)
	for len(r) > 2 && textW(string(r)+"…", 12, false) > 58 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (f *focusView) tickClock() {
	for i := range f.shown {
		if f.shown[i].end.IsZero() {
			f.shown[i].tickTimer()
		}
	}
	var parts []string
	if (f.state == Working || f.state == Starting || f.state == NeedsYou) && !f.turnFrom.IsZero() {
		parts = append(parts, elapsed(time.Since(f.turnFrom)))
	}
	if c := ctxText(f.ctxUsed, f.ctxWindow); c != "" {
		parts = append(parts, c)
	}
	if f.cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", f.cost))
	}
	f.clock.SetText(strings.Join(parts, "  ·  "))
}

type toolRun struct {
	node, head, headL, body *caution.Node
	tools                   []string
	failed, folded, open    bool
}

func (f *focusView) addEntry(e Entry) shown {
	if e.Kind == EntryTool {
		return f.addTool(e)
	}
	if f.run != nil {
		f.fold(f.run)
		f.run = nil
	}
	gutter := caution.Label("").FontSize(12).Weight(600).W(72)
	body := caution.VStack().Gap(4).Fill(1).Align("stretch")
	switch e.Kind {
	case EntryUser:
		gutter.SetText("You").SetColor("$accent")
		body.Add(caution.RichText(caution.Span{Text: e.Text}).FontSize(14).Selectable())
		row := caution.HStack().Gap(12).Kids(gutter, body)
		f.list.Add(row)
		f.asks = append(f.asks, userRow{row: row, text: e.Text})
		return shown{rev: e.Rev, gutter: gutter, body: body}
	case EntryNote:
		gutter.SetText("")
		body.Add(caution.Label(e.Text).FontSize(12).Italic().Color("$inkFaint"))
	case EntryAssistant:
		gutter.SetText(f.name()).SetColor("$ink")
		body.Add(caution.RichText(markdownSpans(e.Text)...).FontSize(14).Selectable())
	case EntryError:
		gutter.SetText("Error").SetColor("$stFail")
		body.Add(caution.RichText(caution.Span{Text: e.Text}).FontSize(13).Color("$stFail").Selectable())
	}
	f.list.Add(caution.HStack().Gap(12).Kids(gutter, body))
	return shown{rev: e.Rev, gutter: gutter, body: body}
}

func (f *focusView) confirmRestore(n int, text string) {
	v, a := f.v, f.a
	errL := caution.Label("").FontSize(12).Color("$stFail")
	v.showDialog(caution.Dialog("Restore files?").CardSize(480, 200).Kids(
		caution.Label("Puts every file in "+shortPath(a.Snapshot().Meta.Cwd)+" back to how it was just before “"+clip(oneLine(text), 60)+
			"”. Changes made after that, by the agent or by you, are lost. The conversation stays, and the agent is told on your next message.").
			FontSize(13).Color("$inkDim").Wrap().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(16)}),
		errL.Anchor(caution.A{Left: px(20), Right: px(200), Bottom: px(26)}),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(20)}).Kids(
			caution.Button("Cancel").OnClick(v.closeDialog),
			caution.Button("Restore files").Primary().OnClick(func() {
				errL.SetText("Restoring…")
				go func() {
					err := a.Restore(n, text)
					v.s.Update(func() {
						if err != nil {
							errL.SetText(err.Error())
							return
						}
						v.closeDialog()
					})
				}()
			}),
		),
	))
}

func (f *focusView) addTool(e Entry) shown {
	if f.run == nil {
		r := &toolRun{}
		r.body = caution.VStack().Gap(2).Align("stretch")
		r.headL = caution.Label("").FontSize(12).Color("$inkDim").Anchor(caution.A{Left: px(18), Right: px(0), CenterY: px(0)})
		r.head = caution.Panel().H(24).Kids(
			icon(icCheck, 12, "$stReady").Anchor(caution.A{Left: px(0), CenterY: px(0)}),
			r.headL,
			caution.Glass().HoverFill("$hover").Anchor(fill()).Tip("Show every step").
				OnPick(func(_, _ float64, _ *caution.Node) { f.unfold(r) }),
		)
		r.node = caution.VStack().Gap(2).Align("stretch").Kids(r.body)
		f.list.Add(r.node)
		f.run = r
	}
	r := f.run
	r.tools = append(r.tools, e.Tool)
	text := e.Text
	if text == "" {
		text = e.Tool
	}
	mark := newMark(icCircle, 12, "$inkFaint")
	name := caution.Label(toolName(e.Tool)).FontSize(12).Color("$inkFaint")
	timer := caution.Label("").FontSize(12).Color("$inkFaint")
	idx := len(f.shown)
	row := caution.Panel().H(22).Kids(
		mark.box.Anchor(caution.A{Left: px(-2), CenterY: px(0)}),
		name.Anchor(caution.A{Left: px(20), CenterY: px(0)}),
		caution.Label(clip(text, 96)).FontSize(12).Mono().Color("$inkDim").Clips().
			Anchor(caution.A{Left: px(84), Right: px(60), CenterY: px(0)}),
		timer.Anchor(caution.A{Right: px(0), CenterY: px(0)}),
		caution.Glass().HoverFill("$hover").Anchor(fill()).Tip("Show the output").
			OnPick(func(_, _ float64, _ *caution.Node) { f.toggle(idx) }),
	)
	slot := caution.VStack().Align("stretch")
	r.body.Add(caution.VStack().Gap(4).Align("stretch").Kids(row, slot))
	sh := shown{rev: e.Rev, gutter: name, body: slot, mark: mark, timer: timer, run: r}
	f.refreshEntry(&sh, e)
	return sh
}

func (f *focusView) fold(r *toolRun) {
	if len(r.tools) < 3 || r.failed || r.folded {
		return
	}
	seen := map[string]bool{}
	var kinds []string
	for _, t := range r.tools {
		if !seen[t] {
			seen[t] = true
			kinds = append(kinds, t)
		}
	}
	r.headL.SetText(fmt.Sprintf("%d steps  ·  %s  ›", len(r.tools), strings.Join(kinds, ", ")))
	r.body.Remove()
	r.node.Add(r.head)
	r.folded = true
}

func (f *focusView) unfold(r *toolRun) {
	if !r.folded {
		return
	}
	r.head.Remove()
	r.node.Add(r.body)
	r.folded = false
}

func (f *focusView) refreshEntry(sh *shown, e Entry) {
	sh.rev = e.Rev
	if e.Kind != EntryTool {
		return
	}
	sh.start, sh.end = e.Start, e.End
	sh.tickTimer()
	switch {
	case e.Failed:
		sh.gutter.SetColor("$stFail")
		sh.mark.set(icFail, "$stFail")
		if sh.run != nil {
			sh.run.failed = true
			f.unfold(sh.run)
		}
	case e.Detail != "" || e.Rev > 0:
		sh.mark.set(icCheck, "$stReady")
	default:
		sh.mark.set(icLoader, "$stWork")
	}
	if e.Failed && !sh.open && e.Detail != "" {
		f.showDetail(sh, e)
	}
}

func (f *focusView) toggle(i int) {
	if i >= len(f.shown) {
		return
	}
	sh := &f.shown[i]
	es := f.a.Snapshot().Entries
	if sh.open {
		sh.detail.Remove()
		sh.open = false
		return
	}
	if i < len(es) {
		f.showDetail(sh, es[i])
	}
}

func (f *focusView) showDetail(sh *shown, e Entry) {
	if e.Detail == "" {
		return
	}
	var spans []caution.Span
	for _, ln := range strings.Split(trimLines(e.Detail, 14, 1600), "\n") {
		c := "$inkDim"
		switch {
		case e.Failed:
			c = "$stFail"
		case strings.HasPrefix(ln, "+"):
			c = "$stReady"
		case strings.HasPrefix(ln, "-"):
			c = "$stFail"
		}
		spans = append(spans, caution.Span{Text: ln + "\n", Mono: true, Color: c})
	}
	sh.detail = caution.HStack().Kids(caution.Panel().W(76),
		caution.VStack().Pad(8).Bg("$codeBg").Radius(6).Align("stretch").Fill(1).Kids(
			caution.RichText(spans...).FontSize(11).Selectable()))
	sh.body.Add(sh.detail)
	sh.open = true
}

func (f *focusView) buildCard(s Snapshot) {
	f.card.Clear()
	if s.State != NeedsYou || len(s.Pending) == 0 {
		f.card.H(0)
		return
	}
	p := s.Pending[0]
	a := f.a
	if len(p.Questions) > 0 || p.Tool == "ExitPlanMode" {
		label := f.name() + " has a question."
		if p.Tool == "ExitPlanMode" {
			label = f.name() + " has a plan for you to review."
		}
		f.card.H(56).Kids(
			caution.Label(label).FontSize(13).Weight(600).Color("$stNeed").
				Anchor(caution.A{Left: px(24), CenterY: px(0)}),
			caution.Button("Show").Primary().Anchor(caution.A{Right: px(20), CenterY: px(0)}).
				OnClick(func() { f.ask(p) }),
		)
		if f.asked != p.ReqID {
			f.ask(p)
		}
		return
	}
	detail := p.Summary
	if c, ok := p.Input["command"].(string); ok && p.Tool == "Bash" {
		detail = c
	}
	f.card.H(64).Kids(
		caution.Panel().W(3).Bg("$stNeed").Anchor(caution.A{Left: px(0), Top: px(0), Bottom: px(0)}),
		caution.HStack().Gap(12).Align("center").Anchor(caution.A{Left: px(20), Right: px(330), CenterY: px(0)}).Kids(
			icon(icHand, 16, "$stNeed"),
			caution.Label(f.name()+" wants to "+plainTool(p.Tool, "")).FontSize(13).Weight(600).Color("$stNeed"),
			codeChip(clip(oneLine(detail), 90), 12),
		),
		caution.HStack().Gap(6).Align("center").Anchor(caution.A{Right: px(20), CenterY: px(0)}).Kids(
			caution.Button("Deny  D").OnClick(func() { a.Answer(p.ReqID, false, false, nil) }),
			f.always(p),
			caution.Button("Allow  A").Primary().OnClick(func() { a.Answer(p.ReqID, true, false, nil) }),
		),
	)
}

func (f *focusView) ask(p *Pending) {
	f.asked = p.ReqID
	a := f.a
	v := f.v
	if p.Tool == "ExitPlanMode" {
		plan, _ := p.Input["plan"].(string)
		d := caution.Dialog("Review the plan").CardSize(720, 560).Kids(
			caution.Scroll().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(12), Bottom: px(64)}).Kids(
				caution.VStack().Align("stretch").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)}).Kids(caution.RichText(markdownSpans(plan)...).FontSize(14).Selectable()),
			),
			caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(18)}).Kids(
				caution.Button("Keep planning").OnClick(func() { v.closeDialog(); a.Answer(p.ReqID, false, false, nil) }),
				caution.Button("Approve").Primary().OnClick(func() { v.closeDialog(); a.Answer(p.ReqID, true, false, nil) }),
			),
		)
		v.showDialog(d)
		return
	}

	form := newAskForm(p.Questions, func(answers map[string]string) {
		v.closeDialog()
		a.Answer(p.ReqID, true, false, answers)
	})
	send := caution.Button("Answer  Enter")
	var allCards [][]*optionCard
	var fields []*caution.Node
	form.onClearText = func(qi int) { fields[qi].SetValueNow("") }
	form.onChange = func() {
		for qi, cards := range allCards {
			for _, c := range cards {
				c.set(form.picked(qi, c.label))
			}
		}
		send.SetProp("primary", form.ready())
	}
	list := caution.VStack().Gap(10).Align("stretch").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	h := 120.0
	for qi, q := range p.Questions {
		if qi > 0 {
			list.Add(caution.Panel().H(8))
			h += 18
		}
		if q.Header != "" {
			list.Add(caution.Label(strings.ToUpper(q.Header)).FontSize(11).Weight(650).Color("$inkFaint"))
			h += 24
		}
		list.Add(caution.Label(q.Question).FontSize(14).Weight(600).Wrap())
		h += float64(len(wrapLines(q.Question, 14, askTextW+40)))*20 + 10
		var cards []*optionCard
		for _, o := range q.Options {
			c := newOptionCard(o, q.MultiSelect)
			cards = append(cards, c)
			c.onPick = func() { form.pick(qi, c.label) }
			list.Add(c.node)
			h += c.h + 10
		}
		allCards = append(allCards, cards)
		other := caution.TextField("").Placeholder("Or type an answer")
		other.OnInput(func(s string) { form.typed(qi, s) })
		other.OnSubmit(func(string) { form.submit() })
		fields = append(fields, other)
		list.Add(other)
		h += 40
	}
	send.OnClick(form.submit)
	d := caution.Dialog(f.name()+" is asking").CardSize(askW, min(h, 680)).Kids(
		caution.Scroll().Anchor(caution.A{Left: px(20), Right: px(20), Top: px(12), Bottom: px(64)}).Kids(list),
		caution.HStack().Gap(8).Anchor(caution.A{Right: px(20), Bottom: px(18)}).Kids(
			caution.Button("Skip").Tip("Decline to answer").OnClick(func() { v.closeDialog(); a.Answer(p.ReqID, false, false, nil) }),
			send,
		),
	)
	v.showDialog(d)
	v.askSend = form.submit
}

type askForm struct {
	qs          []Question
	sel         []map[string]bool
	text        []string
	send        func(map[string]string)
	sent        bool
	onClearText func(int)
	onChange    func()
}

func newAskForm(qs []Question, send func(map[string]string)) *askForm {
	f := &askForm{qs: qs, send: send, text: make([]string, len(qs))}
	for range qs {
		f.sel = append(f.sel, map[string]bool{})
	}
	return f
}

func (f *askForm) pick(qi int, label string) {
	if f.text[qi] != "" {
		f.text[qi] = ""
		if f.onClearText != nil {
			f.onClearText(qi)
		}
	}
	if f.qs[qi].MultiSelect {
		f.sel[qi][label] = !f.sel[qi][label]
	} else {
		f.sel[qi] = map[string]bool{label: true}
	}
	f.changed()
}

func (f *askForm) typed(qi int, s string) {
	f.text[qi] = s
	if strings.TrimSpace(s) != "" {
		f.sel[qi] = map[string]bool{}
	}
	f.changed()
}

func (f *askForm) picked(qi int, label string) bool { return f.sel[qi][label] }

func (f *askForm) answer(qi int) string {
	if x := strings.TrimSpace(f.text[qi]); x != "" {
		return x
	}
	var sel []string
	for _, o := range f.qs[qi].Options {
		if f.sel[qi][o.Label] {
			sel = append(sel, o.Label)
		}
	}
	return strings.Join(sel, ", ")
}

func (f *askForm) ready() bool {
	for qi := range f.qs {
		if f.answer(qi) == "" {
			return false
		}
	}
	return true
}

func (f *askForm) submit() {
	if f.sent || !f.ready() {
		return
	}
	f.sent = true
	answers := map[string]string{}
	for qi, q := range f.qs {
		answers[q.Question] = f.answer(qi)
	}
	f.send(answers)
}

func (f *askForm) changed() {
	if f.onChange != nil {
		f.onChange()
	}
}

func (f *focusView) restyle() {
	if f.bar != nil {
		f.bar.Remove()
		f.bar = nil
	}
	if f.state != Working && f.state != Starting {
		return
	}
	r, g, b := tokenRGB(f.v.set, stateToken(f.state))
	tint := map[string]float64{"tintR": r, "tintG": g, "tintB": b}
	f.bar = caution.Shader(caution.FX{Frag: barFrag, Animate: true, Uniforms: tint}).H(2).
		Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	f.statusBar.Add(f.bar)
}

func (f *focusView) hint() string {
	if providerOf(f.a.Snapshot().Meta) == "claude" {
		return "Message " + f.name() + ", / for commands and skills, @ for a file"
	}
	return "Message " + f.name() + ", @ for a file"
}

func (f *focusView) name() string {
	if f.a == f.v.hub.Dispatcher {
		return "Dispatcher"
	}
	return providerName(f.a.Snapshot().Meta.Provider)
}

func (f *focusView) always(p *Pending) *caution.Node {
	if !p.CanAlways {
		return caution.Panel().W(0)
	}
	return caution.Button("Always  " + shiftLabel + "A").Tip("Allow this for the rest of the session").
		OnClick(func() { f.a.Answer(p.ReqID, true, true, nil) })
}

const (
	askW     = 680.0
	askTextW = askW - 40 - 14 - 56
)

type optionCard struct {
	node, bg, mark *caution.Node
	label          string
	multi          bool
	h              float64
	onPick         func()
}

func newOptionCard(o Option, multi bool) *optionCard {
	c := &optionCard{label: o.Label, multi: multi}
	c.h = 18 + float64(len(wrapLines(o.Label, 13, askTextW)))*19
	if o.Description != "" {
		c.h += 2 + float64(len(wrapLines(o.Description, 12, askTextW)))*17
	}
	c.bg = caution.Panel().Radius(8).Bg("$panelAlt").Border("$edgeSoft", 1).Anchor(fill())
	c.mark = icon("", 15, "$inkFaint").Anchor(caution.A{Left: px(14), Top: px(10)})
	text := caution.VStack().Gap(2).Align("stretch").Anchor(caution.A{Left: px(40), Right: px(14), Top: px(9)}).Kids(
		caution.Label(o.Label).FontSize(13).Weight(600).Wrap())
	if o.Description != "" {
		text.Add(caution.Label(o.Description).FontSize(12).Color("$inkDim").Wrap())
	}
	c.node = caution.Panel().H(c.h).NoFade().Kids(
		c.bg, c.mark, text,
		caution.Glass().HoverFill("$hover").Anchor(fill()).
			OnPick(func(_, _ float64, _ *caution.Node) {
				if c.onPick != nil {
					c.onPick()
				}
			}),
	)
	c.set(false)
	return c
}

func (c *optionCard) set(on bool) {
	glyph := icCircle
	if c.multi {
		glyph = icSquare
	}
	if on {
		c.mark.SetText(icCheck).SetColor("$accent")
		c.bg.Bg("$control").Border("$accent", 1)
		return
	}
	c.mark.SetText(glyph).SetColor("$inkFaint")
	c.bg.Bg("$panelAlt").Border("$edgeSoft", 1)
}

func wrapLines(s string, size, width float64) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, w := range strings.Fields(para) {
			next := w
			if line != "" {
				next = line + " " + w
			}
			if line != "" && textW(next, size, false)*1.05 > width {
				lines = append(lines, line)
				line = w
				continue
			}
			line = next
		}
		lines = append(lines, line)
	}
	return lines
}
