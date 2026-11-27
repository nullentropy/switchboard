package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	caution "github.com/nullentropy/caution"
)

type tile struct {
	v *View
	a *Agent

	icon                                                                 *stateMark
	node, title, agent, badge, status, trail, where, meta, action, bar   *caution.Node
	card, row, fix, edge, mini, miniIcon, miniTitle, miniAgent, miniLine *caution.Node
	trailRow, miniBadge, miniMeta                                        *caution.Node
	statusFX                                                             glint

	snap     Snapshot
	state    State
	pendKey  string
	trailKey string
	selected bool
	turnFrom time.Time
	cost     float64
}

func icon(glyph string, size float64, color string) *caution.Node {
	return caution.Label(glyph).Font("Lucide").FontSize(size).Color(color)
}

func textW(s string, size float64, mono bool) float64 {
	per := size * 0.56
	if mono {
		per = size * 0.62
	}
	return float64(len([]rune(expandTabs(s)))) * per
}

func kbd(keys string) *caution.Node {
	return caution.Panel().W(textW(keys, 10, true)+10).H(18).Bg("$panelAlt").Border("$edge", 1).Radius(4).Kids(
		caution.Label(keys).FontSize(10).Mono().Color("$inkDim").Anchor(caution.A{CenterX: px(0), CenterY: px(0)}))
}

func chip(text, glyph string) *caution.Node {
	return caution.Panel().W(textW(text, 11, false)+30).H(20).Bg("$panelAlt").Radius(5).Kids(
		icon(glyph, 11, "$inkFaint").Anchor(caution.A{Left: px(7), CenterY: px(0)}),
		caution.Label(text).FontSize(11).Weight(550).Color("$inkDim").Anchor(caution.A{Left: px(22), CenterY: px(0)}))
}

func codeChip(text string, size float64) *caution.Node {
	return caution.Panel().W(textW(text, size, true) + 14).H(size + 12).Bg("$codeBg").Radius(5).Kids(
		caution.Label(text).FontSize(size).Mono().Color("$ink").Anchor(caution.A{Left: px(7), CenterY: px(0)}))
}

func iconButton(glyph, tip string, fn func()) *caution.Node {
	return caution.Panel().W(28).H(28).Radius(7).Bg("$panelAlt").Border("$edgeSoft", 1).Kids(
		icon(glyph, 14, "$inkDim").Anchor(caution.A{CenterX: px(0), CenterY: px(0)}),
		caution.Glass().Anchor(fill()).Tip(tip).OnPick(func(_, _ float64, _ *caution.Node) { fn() }),
	)
}

const tileRadius = 12.0

func (v *View) sessionMenu(a *Agent) []caution.ContextItem {
	return []caution.ContextItem{
		{Title: "Open", OnPick: func() { v.select_(a); v.openFocus(a) }},
		{Title: "Rename…", OnPick: func() { v.openRename(a) }},
		{Sep: true},
		{Title: "Interrupt", OnPick: a.Interrupt},
		{Title: "End session…", OnPick: func() { v.confirmEnd(a) }},
		{Sep: true},
		{Title: "Remove…", OnPick: func() { v.confirmRemove(a) }},
	}
}

func newTile(v *View, a *Agent) *tile {
	t := &tile{v: v, a: a, state: -1}
	open := func() { v.select_(a); v.openFocus(a) }
	t.icon = newMark(icLoader, 18, "$stWork")
	t.title = caution.Label("").FontSize(14).Weight(620)
	t.agent = caution.HStack()
	t.badge = caution.HStack()
	t.status = caution.Label("").FontSize(13).Clips().Fill(1)
	t.statusFX.n = t.status
	t.trail = caution.Label("").FontSize(12).Mono().Color("$inkFaint").Clips().Fill(1)
	t.where = caution.Label("").FontSize(11).Color("$inkFaint").Clips().Fill(1)
	t.meta = caution.Label("").FontSize(11).Mono().Color("$inkFaint")
	t.action = caution.HStack().Gap(6).Align("center")
	t.fix = caution.HStack().Gap(8).Align("center").H(0)
	t.trailRow = caution.HStack().Align("center").H(18).Clips().Kids(t.trail)

	content := caution.VStack().Gap(8).Pad(14).Align("stretch").Anchor(fill()).Kids(
		caution.HStack().Gap(8).Align("center").Kids(t.icon.box, t.title, t.agent, caution.Panel().Fill(1), t.badge),
		caution.HStack().Align("center").Kids(t.status),
		t.fix,
		t.trailRow,
		caution.HStack().Gap(10).Align("center").H(28).Kids(t.where, t.meta, t.action),
	)
	ctx := v.sessionMenu(a)
	t.edge = caution.Panel().W(3)
	t.node = caution.Panel().H(tileH).Bg("$panel").Radius(tileRadius).Border("$edgeSoft", 1).Clips().
		Kids(caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { open() }), t.edge, content).
		Context(ctx...)

	t.miniIcon = icon(icCheck, 15, "$stReady")
	t.miniTitle = caution.Label("").FontSize(13).Weight(600)
	t.miniAgent = icon(icSparkles, 11, "$inkFaint")
	t.miniLine = caution.Label("").FontSize(12).Color("$inkDim").Clips().Fill(1)
	t.miniBadge = caution.HStack()
	t.miniMeta = caution.Label("").FontSize(11).Mono().Color("$inkFaint")
	t.mini = caution.Panel().H(64).Bg("$panel").Radius(10).Border("$edgeSoft", 1).Clips().Kids(
		caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { open() }),
		caution.VStack().Gap(5).Anchor(caution.A{Left: px(12), Right: px(12), CenterY: px(0)}).Align("stretch").Kids(
			caution.HStack().Gap(7).Align("center").Kids(t.miniIcon, t.miniTitle, t.miniAgent, caution.Panel().Fill(1), t.miniMeta, t.miniBadge),
			caution.HStack().Kids(caution.Panel().W(15), t.miniLine),
		),
	).Context(ctx...)
	return t
}

func (t *tile) update(s Snapshot) {
	t.snap = s
	stateChanged := s.State != t.state
	t.state = s.State
	t.turnFrom = s.TurnFrom
	t.cost = s.Cost
	tok := "$" + stateToken(s.State)
	t.icon.set(stateIcon(s.State), tok)
	t.title.SetText(clip(s.Meta.Title, 26))
	if stateChanged || t.agent.Children() == nil {
		t.agent.Clear()
		t.agent.Add(chip(providerName(s.Meta.Provider), agentIcon(s.Meta.Provider)))
	}
	text, color := statusLine(s)
	t.status.SetText(clip(text, 58)).SetColor(color)
	t.statusFX.set(t.v.set, clip(text, 58), 13, false, thinking(s))
	where := filepath.Base(s.Meta.Cwd)
	if s.Branch != "" {
		where += " / " + s.Branch
	}
	if s.Meta.Goal != "" {
		where = s.Meta.Goal + "  ·  " + where
	}
	t.where.SetText(clip(where, 40))
	t.updateTrail(s)

	t.miniIcon.SetText(stateIcon(s.State)).SetColor(tok)
	t.miniTitle.SetText(clip(s.Meta.Title, 24))
	t.miniAgent.SetText(agentIcon(s.Meta.Provider))
	t.miniLine.SetText(clip(text, 54))

	key := ""
	if len(s.Pending) > 0 {
		key = s.Pending[0].ReqID
	}
	if stateChanged || key != t.pendKey {
		t.pendKey = key
		t.buildAction(s)
		t.buildFix(s)
		t.card, t.row = nil, nil
		if s.State == NeedsYou && len(s.Pending) > 0 {
			t.card = t.inboxCard(s)
			t.row = t.inboxRow(s)
		}
		t.restyle()
	}
	t.tickClock()
}

func fixCommand(err string) (string, bool) {
	if _, cmd, ok := strings.Cut(err, "`"); ok {
		if cmd, _, ok = strings.Cut(cmd, "`"); ok {
			return cmd, true
		}
	}
	return "", false
}

func (t *tile) buildFix(s Snapshot) {
	t.fix.Clear()
	cmd, ok := fixCommand(s.Err)
	if s.State != Failed || !ok {
		t.fix.H(0)
		t.trailRow.H(18)
		t.node.H(tileH)
		return
	}
	t.trailRow.H(0)
	t.node.H(156)
	t.fix.H(26).Kids(
		caution.Label("Run in a terminal").FontSize(12).Color("$inkDim"),
		caution.Panel().W(textW(cmd, 12, true)+16).H(24).Bg("$codeBg").Radius(5).Kids(
			caution.RichText(caution.Span{Text: cmd, Mono: true}).FontSize(12).Selectable().
				Anchor(caution.A{Left: px(8), CenterY: px(0)})),
		caution.Label("then retry").FontSize(12).Color("$inkDim"),
	)
}

func (t *tile) setBadge(n int) {
	t.badge.Clear()
	t.miniBadge.Clear()
	if n >= 1 && n <= 9 {
		t.badge.Add(kbd(fmt.Sprintf("%s%d", modLabel, n)))
		t.miniBadge.Add(kbd(fmt.Sprintf("%s%d", modLabel, n)))
	}
}

func (t *tile) setSelected(on bool) {
	if t.selected != on {
		t.selected = on
		t.restyle()
	}
}

func (t *tile) setTarget(on bool) {
	for _, n := range []*caution.Node{t.card, t.row} {
		if n == nil {
			continue
		}
		if on {
			n.Border("$accent", 2)
		} else {
			n.Border("$edgeSoft", 1)
		}
	}
}

func toolPhase(tool, text string) string {
	switch tool {
	case "Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch":
		return "phRead"
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "Delete", "Move":
		return "phEdit"
	case "Bash":
		if commandStation(text) == "Test" {
			return "phTest"
		}
		return "phRun"
	}
	return "phThink"
}

func trailLabel(e Entry) string {
	switch e.Tool {
	case "Read", "Edit", "Write", "MultiEdit", "Delete", "Move":
		return strings.ToLower(e.Tool) + " " + filepath.Base(e.Text)
	case "Bash":
		return e.Text
	case "Grep", "Glob":
		return "search " + e.Text
	}
	return strings.ToLower(e.Tool)
}

func (t *tile) updateTrail(s Snapshot) {
	var steps []string
	for i := len(s.Entries) - 1; i >= 0 && len(steps) < 4; i-- {
		e := s.Entries[i]
		if e.Kind != EntryTool {
			continue
		}
		step := trailLabel(e)
		if e.Failed {
			step += " ✗"
		}
		steps = append(steps, step)
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	for len(steps) > 1 && len([]rune(strings.Join(steps, "  ›  "))) > 52 {
		steps = steps[1:]
	}
	line := strings.Join(steps, "  ›  ")
	if line == "" {
		line = "No steps yet"
	}
	t.trail.SetText(clip(line, 52))
}

func clipLeft(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return "…" + string(r[len(r)-n+1:])
	}
	return s
}

func statusLine(s Snapshot) (string, string) {
	text, color := statusText(s)
	if n := len(s.Queued); n > 0 {
		text = map[bool]string{true: "1 message pending", false: fmt.Sprintf("%d messages pending", n)}[n == 1] + "  ·  " + text
	}
	return text, color
}

func statusText(s Snapshot) (string, string) {
	if s.State == NeedsYou && len(s.Pending) > 0 {
		p := s.Pending[0]
		switch {
		case len(p.Questions) > 0:
			return "Asks: " + oneLine(p.Questions[0].Question), "$stNeed"
		case p.Tool == "ExitPlanMode":
			return "Has a plan for you to review", "$stNeed"
		}
		return "Wants to " + plainTool(p.Tool, p.Summary), "$stNeed"
	}
	switch s.State {
	case Failed:
		if _, ok := fixCommand(s.Err); ok {
			first, _, _ := strings.Cut(s.Err, ". ")
			return strings.TrimSuffix(first, ".") + ".", "$stFail"
		}
		return oneLine(s.Err), "$stFail"
	case Waiting:
		return "Waiting for usage limit, resumes " + resumeLabel(s.Meta.ResumeAt, clock()), "$stNeed"
	case Working, Starting:
		if h := helperLine(s.Subs); h != "" {
			return h, "$ink"
		}
		if s.Activity == "" {
			return "Starting", "$inkDim"
		}
		return plainActivity(s.Activity), "$ink"
	case Background:
		return waitingOn(s.Subs), "$stBg"
	}
	if s.State == Interrupted {
		if s.Meta.Last == "background" {
			return "Its background work stopped when Switchboard quit. A message picks it up again.", "$stNeed"
		}
		return "Interrupted when Switchboard quit mid-turn. A message picks it up again.", "$stNeed"
	}
	if r := lastReply(s.Entries); r != "" {
		return oneLine(plain(r)), "$inkDim"
	}
	if s.State == Stopped {
		return "Ended. A message picks it up again.", "$inkFaint"
	}
	return "Waiting for your first message", "$inkFaint"
}

func helperLine(subs []Sub) string {
	for i := len(subs) - 1; i >= 0; i-- {
		if s := subs[i]; !s.BG && s.Activity != "" {
			return "Helper " + s.Desc + ": " + plainActivity(s.Activity)
		}
	}
	return ""
}

func plainActivity(act string) string {
	tool, rest, _ := strings.Cut(act, " ")
	if tool == "Thinking" || tool == "Writing" || tool == "Starting" {
		return act
	}
	s := plainTool(tool, rest)
	return strings.ToUpper(s[:1]) + s[1:]
}

func plainTool(tool, sum string) string {
	verb := map[string]string{
		"Read": "read", "Grep": "search for", "Glob": "find files", "Edit": "edit", "MultiEdit": "edit", "Write": "write",
		"Bash": "run", "WebFetch": "fetch", "WebSearch": "search the web for", "Task": "start a helper:", "Agent": "start a helper:",
	}[tool]
	if verb == "" {
		verb = "use " + tool
	}
	return strings.TrimSpace(verb + " " + sum)
}

func previewText(es []Entry) string {
	s := previewRaw(es)
	if r := []rune(s); len(r) > 170 {
		s = string(r[:169]) + "…"
	}
	return s
}

func previewRaw(es []Entry) string {
	for i := len(es) - 1; i >= 0; i-- {
		switch es[i].Kind {
		case EntryAssistant:
			return lastLines(plain(es[i].Text), 3)
		case EntryUser:
			return "You: " + lastLines(userLine(es[i]), 3)
		case EntryError:
			return lastLines(es[i].Text, 3)
		}
	}
	return ""
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	var keep []string
	for _, l := range ls {
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, "\n")
}

func (t *tile) buildAction(s Snapshot) {
	t.action.Clear()
	a := t.a
	switch s.State {
	case Working, Starting:
		t.action.Add(iconButton(icPause, "Interrupt this turn (.)", a.Interrupt))
	case Ready, Stopped, Interrupted, Background:
		t.action.Add(caution.Button("Reply").Tip("Open and write the next message (Enter)").OnClick(func() { t.v.openFocus(a) }))
	case Failed:
		t.action.Add(iconButton(icRetry, "Send the last message again", func() { go retry(a) }))
	case Waiting:
		t.action.Add(caution.HStack().Gap(6).Align("center").Kids(
			iconButton(icSquare, "Stop waiting for the limit", func() { go a.CancelWait() }),
			caution.Button("Resume now").Tip("Send the continue message now").OnClick(func() { go a.ResumeNow() }),
		))
	}
}

func retry(a *Agent) {
	s := a.Snapshot()
	for i := len(s.Entries) - 1; i >= 0; i-- {
		if s.Entries[i].Kind == EntryUser {
			a.SendImages(s.Entries[i].Text, s.Entries[i].Images)
			return
		}
	}
}

func (t *tile) inboxCard(s Snapshot) *caution.Node {
	a := t.a
	p := s.Pending[0]
	var ask *caution.Node
	var buttons []*caution.Node
	switch {
	case len(p.Questions) > 0 || p.Tool == "ExitPlanMode":
		text, _ := statusLine(s)
		ask = caution.Label(clip(text, 90)).FontSize(13).Color("$ink")
		buttons = []*caution.Node{
			caution.Button("Answer  Enter").Primary().OnClick(func() { t.v.openFocus(a) }),
		}
	default:
		detail := p.Summary
		if c, ok := p.Input["command"].(string); ok {
			detail = oneLine(c)
		}
		ask = caution.HStack().Gap(8).Align("center").Kids(
			caution.Label("wants to "+plainTool(p.Tool, "")).FontSize(13).Color("$inkDim"),
			codeChip(clip(detail, 80), 12),
		)
		buttons = []*caution.Node{
			caution.Button("Deny  D").OnClick(func() { a.Answer(p.ReqID, false, false, nil) }),
		}
		if p.CanAlways {
			buttons = append(buttons,
				caution.Button("Always  "+shiftLabel+"A").Tip("Allow this for the rest of the session").OnClick(func() { a.Answer(p.ReqID, true, true, nil) }))
		}
		buttons = append(buttons, caution.Button("Allow  A").Primary().OnClick(func() { a.Answer(p.ReqID, true, false, nil) }))
	}
	waited := ""
	if !s.TurnFrom.IsZero() {
		waited = elapsed(time.Since(s.TurnFrom))
	}
	return caution.Panel().H(76).Bg("$needBg").Radius(12).Border("$edgeSoft", 1).Clips().Kids(
		caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { t.v.select_(a); t.v.openFocus(a) }),
		caution.Panel().W(4).Bg("$stNeed").RadiusCorners(10, 0, 0, 10).Anchor(caution.A{Left: px(2), Top: px(2), Bottom: px(2)}),
		caution.HStack().Gap(14).Align("center").Anchor(caution.A{Left: px(18), Right: px(16), Top: px(0), Bottom: px(0)}).Kids(
			icon(icHand, 18, "$stNeed"),
			caution.VStack().Gap(6).Fill(1).Kids(
				caution.HStack().Gap(8).Align("center").Kids(
					caution.Label(clip(s.Meta.Title, 28)).FontSize(14).Weight(620),
					chip(providerName(s.Meta.Provider), agentIcon(s.Meta.Provider)),
					caution.Label(clip(filepath.Base(s.Meta.Cwd), 24)).FontSize(11).Color("$inkFaint"),
					caution.Label(waited).FontSize(11).Mono().Color("$inkFaint"),
				),
				ask,
			),
			caution.HStack().Gap(6).Align("center").Kids(buttons...),
		),
	)
}

func rowAsk(p *Pending) *caution.Node {
	if len(p.Questions) > 0 {
		return caution.Label("asks: " + clip(p.Questions[0].Question, 60)).FontSize(13).Color("$inkDim")
	}
	if p.Tool == "ExitPlanMode" {
		return caution.Label("has a plan to review").FontSize(13).Color("$inkDim")
	}
	detail := p.Summary
	if c, ok := p.Input["command"].(string); ok {
		detail = oneLine(c)
	}
	return caution.HStack().Gap(8).Align("center").Kids(
		caution.Label("wants to "+plainTool(p.Tool, "")).FontSize(13).Color("$inkDim"),
		codeChip(clip(detail, 60), 12))
}

func (t *tile) inboxRow(s Snapshot) *caution.Node {
	a := t.a
	p := s.Pending[0]
	age := ""
	if !s.TurnFrom.IsZero() {
		age = elapsed(time.Since(s.TurnFrom))
	}
	var right []*caution.Node
	if len(p.Questions) > 0 || p.Tool == "ExitPlanMode" {
		right = []*caution.Node{caution.Button("Answer  Enter").Primary().OnClick(func() { t.v.openFocus(a) })}
	} else {
		right = []*caution.Node{
			caution.Button("Deny  D").OnClick(func() { a.Answer(p.ReqID, false, false, nil) }),
			caution.Button("Allow  A").Primary().OnClick(func() { a.Answer(p.ReqID, true, false, nil) }),
		}
	}
	return caution.Panel().H(52).Bg("$needBg").Radius(10).Border("$edgeSoft", 1).Clips().Kids(
		caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { t.v.select_(a); t.v.openFocus(a) }),
		caution.Panel().W(4).Bg("$stNeed").RadiusCorners(8, 0, 0, 8).Anchor(caution.A{Left: px(2), Top: px(2), Bottom: px(2)}),
		caution.HStack().Gap(12).Align("center").Anchor(caution.A{Left: px(16), Right: px(12), Top: px(0), Bottom: px(0)}).Kids(
			icon(icHand, 16, "$stNeed"),
			caution.Label(clip(s.Meta.Title, 22)).FontSize(13).Weight(620),
			icon(agentIcon(s.Meta.Provider), 12, "$inkFaint"),
			rowAsk(p),
			caution.Panel().Fill(1),
			caution.Label(age).FontSize(11).Mono().Color("$inkFaint"),
			caution.HStack().Gap(6).Align("center").Kids(right...),
		),
	)
}

func (t *tile) tickClock() {
	var parts []string
	if t.state == Working || t.state == Starting || t.state == NeedsYou {
		if !t.turnFrom.IsZero() {
			parts = append(parts, elapsed(time.Since(t.turnFrom)))
		}
	}
	if c := ctxText(t.snap.CtxUsed, t.snap.CtxWindow); c != "" {
		parts = append(parts, c)
	}
	if t.cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", t.cost))
	}
	t.meta.SetText(strings.Join(parts, " · "))
	if t.cost > 0 {
		t.miniMeta.SetText(fmt.Sprintf("$%.2f", t.cost))
	}
}

func elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func (t *tile) restyle() {
	bg := "$panel"
	if t.state == Working || t.state == Starting {
		bg = "$workBg"
	}
	edge := bg
	if t.state == Failed {
		edge = "$stFail"
	}
	t.node.Bg(bg)
	title := "$ink"
	if t.state == Ready || t.state == Stopped {
		title = "$inkDim"
	}
	t.title.SetColor(title)
	border, bw := "$edgeSoft", 1.0
	if t.selected {
		border, bw = "$accent", 2.0
	}
	t.node.Border(border, bw)
	t.mini.Border(border, bw)
	t.edge.Bg(edge).Anchor(caution.A{Left: px(bw), Top: px(tileRadius), Bottom: px(tileRadius)})
	if t.bar != nil {
		t.bar.Remove()
		t.bar = nil
	}
	if t.state == Working || t.state == Starting {
		r, g, b := tokenRGB(t.v.set, "stWork")
		t.bar = caution.Shader(caution.FX{Frag: barFrag, Animate: true, Uniforms: map[string]float64{"tintR": r, "tintG": g, "tintB": b}}).H(2).
			Anchor(caution.A{Left: px(tileRadius), Right: px(tileRadius), Top: px(bw)})
		t.node.Add(t.bar)
	}
}
