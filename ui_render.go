package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"

	"switchboard/platform"
	"switchboard/uiscript"
)

const uiMaxNodes = 2000

type uiActor interface {
	uiOpenSession(a *Agent)
	uiNewSession()
	uiFocusInput()
	uiOpenPalette()
	uiNextWaiting()
	uiShowBoard()
	uiOpenSettings()
	uiAnswer(a *Agent, allow, always bool)
	uiInterrupt(a *Agent)
	uiFilterGoal(goal string)
	uiClearFilter()
	uiStopSession(a *Agent)
	uiRemoveSession(a *Agent)
	uiMarkGoalDone(goal string)
}

type uiRenderer struct {
	v     *View
	act   uiActor
	nodes int
}

func renderUI(v *View, act uiActor, root *uiscript.Node, data map[string]any, session any) (out *caution.Node, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, fmt.Errorf("the interface failed while rendering: %v", p)
		}
	}()
	if root == nil {
		return nil, fmt.Errorf("the slot is empty")
	}
	r := &uiRenderer{v: v, act: act}
	nodes, err := r.expand(root, uiscript.NewScope(data, session))
	if err != nil {
		return nil, err
	}
	switch len(nodes) {
	case 0:
		return caution.Panel(), nil
	case 1:
		return nodes[0], nil
	}
	return caution.VStack().Gap(0).Align("stretch").Kids(nodes...), nil
}

func uiDo(act uiActor, h *Hub, a uiscript.Action) {
	agent := func() *Agent {
		for _, x := range h.Agents() {
			if x.Meta.ID == a.Session {
				return x
			}
		}
		return nil
	}
	if uiscript.Actions[a.Action] == uiscript.ArgSession {
		if x := agent(); x != nil {
			uiDoSession(act, x, a.Action)
		}
		return
	}
	switch a.Action {
	case "new_session":
		act.uiNewSession()
	case "focus_input":
		act.uiFocusInput()
	case "open_palette":
		act.uiOpenPalette()
	case "next_waiting":
		act.uiNextWaiting()
	case "show_board":
		act.uiShowBoard()
	case "open_settings":
		act.uiOpenSettings()
	case "filter_goal":
		act.uiFilterGoal(a.Goal)
	case "clear_filter":
		act.uiClearFilter()
	case "mark_goal_done":
		act.uiMarkGoalDone(a.Goal)
	}
}

func uiDoSession(act uiActor, x *Agent, action string) {
	switch action {
	case "open_session":
		act.uiOpenSession(x)
	case "allow":
		act.uiAnswer(x, true, false)
	case "deny":
		act.uiAnswer(x, false, false)
	case "always":
		act.uiAnswer(x, true, true)
	case "interrupt":
		act.uiInterrupt(x)
	case "stop_session":
		act.uiStopSession(x)
	case "remove_session":
		act.uiRemoveSession(x)
	}
}

func (r *uiRenderer) add(k int) error {
	r.nodes += k
	if r.nodes > uiMaxNodes {
		return fmt.Errorf("the slot renders more than %d nodes", uiMaxNodes)
	}
	return nil
}

func (r *uiRenderer) count(n *caution.Node) error {
	if err := r.add(1); err != nil {
		return err
	}
	for _, k := range n.Children() {
		if err := r.count(k); err != nil {
			return err
		}
	}
	return nil
}

func (r *uiRenderer) expand(n *uiscript.Node, sc *uiscript.Scope) ([]*caution.Node, error) {
	if n == nil {
		return nil, fmt.Errorf("null node")
	}
	scopes, err := sc.Expand(n)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", n.Type, err)
	}
	var out []*caution.Node
	for _, s := range scopes {
		node, err := r.node(n, s)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

func (r *uiRenderer) node(n *uiscript.Node, sc *uiscript.Scope) (*caution.Node, error) {
	if uiscript.Components[n.Type] {
		node, err := r.component(n, sc)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", n.Type, err)
		}
		if err := r.count(node); err != nil {
			return nil, err
		}
		if err := r.sizeProps(node, n, sc); err != nil {
			return nil, fmt.Errorf("%s: %v", n.Type, err)
		}
		return node, nil
	}
	if !uiscript.NativeTypes[n.Type] {
		return nil, fmt.Errorf("unknown node type %q", n.Type)
	}
	if len(n.Kids) > 0 && !uiscript.Containers[n.Type] {
		return nil, fmt.Errorf("%s cannot hold kids", n.Type)
	}
	text, err := sc.Interp(n.Text)
	if err != nil {
		return nil, fmt.Errorf("%s text: %v", n.Type, err)
	}
	var node, holder *caution.Node
	switch n.Type {
	case "panel":
		node = caution.Panel()
	case "vstack":
		node = caution.VStack()
	case "hstack":
		node = caution.HStack()
	case "grid":
		cols := 1
		if v, ok, err := r.num(sc, n.Props["columns"]); err != nil {
			return nil, fmt.Errorf("grid columns: %v", err)
		} else if ok && v >= 1 {
			cols = int(v)
		}
		node = caution.Grid(gridCols(cols))
	case "scroll":
		holder = caution.VStack().Align("stretch").Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
		node = caution.Scroll().Kids(holder)
	case "glass":
		node = caution.Glass()
	case "label":
		node = caution.Label(text)
	case "richtext":
		node = caution.RichText(caution.Span{Text: text}).Wrap()
	case "button":
		node = caution.Button(text)
	case "progress":
		v, _, err := r.num(sc, n.Props["value"])
		if err != nil {
			return nil, fmt.Errorf("progress value: %v", err)
		}
		node = caution.Progress(math.Max(0, math.Min(1, v)))
	case "image":
		src, err := r.str(sc, n.Props["src"])
		if err != nil {
			return nil, fmt.Errorf("image src: %v", err)
		}
		if !strings.HasPrefix(src, "data:image/") {
			return nil, fmt.Errorf("image src must be a data:image/ URI")
		}
		node = caution.Image(src)
	}
	if err := r.add(1); err != nil {
		return nil, err
	}
	if holder != nil {
		if err := r.add(1); err != nil {
			return nil, err
		}
	}
	if err := r.props(node, holder, n, sc); err != nil {
		return nil, fmt.Errorf("%s: %v", n.Type, err)
	}
	parent := node
	if holder != nil {
		parent = holder
	}
	for _, k := range n.Kids {
		kids, err := r.expand(k, sc)
		if err != nil {
			return nil, err
		}
		for _, c := range kids {
			parent.Add(c)
		}
	}
	if n.OnClick != nil {
		a, err := sc.Action(n.OnClick)
		if err != nil {
			return nil, fmt.Errorf("%s onClick: %v", n.Type, err)
		}
		return r.clickable(node, n.Type, a)
	}
	return node, nil
}

func (r *uiRenderer) clickable(node *caution.Node, typ string, a uiscript.Action) (*caution.Node, error) {
	h := r.v.hub
	act := r.act
	run := func() { uiDo(act, h, a) }
	pick := func(_, _ float64, _ *caution.Node) { run() }
	switch typ {
	case "button":
		return node.OnClick(run), nil
	case "glass":
		return node.OnPick(pick), nil
	}
	glass := caution.Glass().Anchor(fill()).OnPick(pick)
	if typ == "panel" {
		kids := node.Children()
		node.Clear()
		return node.Kids(append([]*caution.Node{glass}, kids...)...), r.add(1)
	}
	wrap := caution.Panel().H(uiNaturalH(node))
	for _, k := range []string{"width", "stack", "anchors"} {
		if v := node.Prop(k); v != nil {
			wrap.SetProp(k, v)
		}
	}
	return wrap.Kids(glass, node.Anchor(fill())), r.add(2)
}

func uiNaturalH(n *caution.Node) float64 {
	if h, ok := n.Prop("height").(float64); ok {
		return h
	}
	pad, _ := n.Prop("padding").(float64)
	gap, ok := n.Prop("spacing").(float64)
	if !ok {
		gap = 8
	}
	kids := n.Children()
	switch n.Type() {
	case wire.Label, wire.RichText:
		size, ok := n.Prop("size").(float64)
		if !ok {
			size = 13
		}
		return math.Ceil(size * 1.25)
	case wire.Button:
		return 26
	case wire.Progress:
		return 6
	case wire.VStack:
		h := 0.0
		for i, k := range kids {
			if i > 0 {
				h += gap
			}
			h += uiNaturalH(k)
		}
		return h + 2*pad
	case wire.HStack:
		h := 0.0
		for _, k := range kids {
			h = math.Max(h, uiNaturalH(k))
		}
		return h + 2*pad
	case wire.Grid:
		cols := 1
		if c, ok := n.Prop("columns").([]map[string]any); ok && len(c) > 0 {
			rg, _ := n.Prop("rowGap").(float64)
			cols = len(c)
			h, row := 0.0, 0.0
			for i, k := range kids {
				row = math.Max(row, uiNaturalH(k))
				if (i+1)%cols == 0 || i == len(kids)-1 {
					if h > 0 {
						h += rg
					}
					h += row
					row = 0
				}
			}
			return h + 2*pad
		}
	}
	return 0
}

func (r *uiRenderer) value(sc *uiscript.Scope, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	return sc.Value(v)
}

func (r *uiRenderer) num(sc *uiscript.Scope, v any) (float64, bool, error) {
	got, err := r.value(sc, v)
	if err != nil || got == nil {
		return 0, false, err
	}
	n, ok := uiscript.Number(got)
	if !ok {
		return 0, false, fmt.Errorf("%v is not a number", got)
	}
	return n, true, nil
}

func (r *uiRenderer) str(sc *uiscript.Scope, v any) (string, error) {
	got, err := r.value(sc, v)
	if err != nil || got == nil {
		return "", err
	}
	s, ok := got.(string)
	if !ok {
		return "", fmt.Errorf("%v is not text", got)
	}
	return s, nil
}

func uiAnchorOf(s string) (caution.A, bool) {
	z := px(0)
	switch s {
	case "fill":
		return fill(), true
	case "center":
		return caution.A{CenterX: z, CenterY: z}, true
	case "top":
		return caution.A{Left: z, Right: z, Top: z}, true
	case "bottom":
		return caution.A{Left: z, Right: z, Bottom: z}, true
	case "left":
		return caution.A{Left: z, Top: z, Bottom: z}, true
	case "right":
		return caution.A{Right: z, Top: z, Bottom: z}, true
	}
	return caution.A{}, false
}

func (r *uiRenderer) sizeProps(node *caution.Node, n *uiscript.Node, sc *uiscript.Scope) error {
	for _, k := range []string{"width", "height", "fill"} {
		v, ok, err := r.num(sc, n.Props[k])
		if err != nil {
			return fmt.Errorf("%s: %v", k, err)
		}
		if !ok {
			continue
		}
		switch k {
		case "width":
			node.W(v)
		case "height":
			node.H(v)
		case "fill":
			node.Fill(v)
		}
	}
	if s, err := r.str(sc, n.Props["anchor"]); err != nil {
		return fmt.Errorf("anchor: %v", err)
	} else if s != "" {
		a, ok := uiAnchorOf(s)
		if !ok {
			return fmt.Errorf("anchor %q is not fill, center, top, bottom, left or right", s)
		}
		node.Anchor(a)
	}
	return nil
}

func (r *uiRenderer) props(node, holder *caution.Node, n *uiscript.Node, sc *uiscript.Scope) error {
	if err := r.sizeProps(node, n, sc); err != nil {
		return err
	}
	inner := node
	if holder != nil {
		inner = holder
	}
	for _, k := range []string{"pad", "gap", "borderWidth", "radius", "fontSize", "weight"} {
		v, ok, err := r.num(sc, n.Props[k])
		if err != nil {
			return fmt.Errorf("%s: %v", k, err)
		}
		if !ok {
			continue
		}
		switch k {
		case "pad":
			inner.Pad(v)
		case "gap":
			if n.Type == "grid" {
				node.ColGap(v).RowGap(v)
			} else {
				inner.Gap(v)
			}
		case "radius":
			node.Radius(v)
		case "fontSize":
			node.FontSize(v)
		case "weight":
			node.Weight(int(v))
		}
	}
	strs := map[string]string{}
	for _, k := range []string{"align", "bg", "color", "border", "tip"} {
		s, err := r.str(sc, n.Props[k])
		if err != nil {
			return fmt.Errorf("%s: %v", k, err)
		}
		strs[k] = s
	}
	if s := strs["align"]; s != "" {
		if !uiscript.Aligns[s] {
			return fmt.Errorf("align %q is not start, center, end or stretch", s)
		}
		inner.Align(caution.Align(s))
	}
	for _, k := range []string{"bg", "color", "border"} {
		if s := strs[k]; s != "" && !uiscript.Hex.MatchString(s) && !uiscript.TokRef.MatchString(s) {
			return fmt.Errorf("%s %q is not a theme token or #rrggbb", k, s)
		}
	}
	if s := strs["bg"]; s != "" {
		node.Bg(s)
	}
	if s := strs["color"]; s != "" {
		node.Color(s)
	}
	bw, hasBW, _ := r.num(sc, n.Props["borderWidth"])
	if s := strs["border"]; s != "" || hasBW {
		if s == "" {
			s = "$edge"
		}
		if !hasBW {
			bw = 1
		}
		node.Border(s, bw)
	}
	if s := strs["tip"]; s != "" {
		node.Tip(s)
	}
	if v, err := r.value(sc, n.Props["mono"]); err != nil {
		return fmt.Errorf("mono: %v", err)
	} else if b, _ := v.(bool); b {
		node.Mono()
	}
	return nil
}

func (r *uiRenderer) agent(n *uiscript.Node, sc *uiscript.Scope) (*Agent, error) {
	id, err := r.str(sc, n.Props["session"])
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, fmt.Errorf("no session given")
	}
	for _, a := range r.v.hub.Agents() {
		if a.Meta.ID == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no session %q", id)
}

func (r *uiRenderer) component(n *uiscript.Node, sc *uiscript.Scope) (*caution.Node, error) {
	switch n.Type {
	case "sb.inbox":
		return r.inbox(sc)
	case "sb.limits":
		return r.limits(), nil
	}
	a, err := r.agent(n, sc)
	if err != nil {
		return nil, err
	}
	s := a.Snapshot()
	switch n.Type {
	case "sb.tile":
		t := newTile(r.v, a)
		t.update(s)
		return t.node, nil
	case "sb.stateDot":
		return newMark(r.v.set, stateIcon(s.State), 14, "$"+stateToken(s.State)).box, nil
	case "sb.sprite":
		return r.sprite(a, s), nil
	case "sb.activity":
		return r.activity(a, s), nil
	}
	return nil, fmt.Errorf("unknown component")
}

func (r *uiRenderer) sprite(a *Agent, s Snapshot) *caution.Node {
	if s.State != Working && s.State != Starting {
		return caution.Panel().W(busyW).H(busyH)
	}
	set := r.v.set
	p := pickSprite(a, s.TurnFrom, set)
	sp, _ := spriteNamed(set, p.name)
	frame := sp.still
	if !p.still {
		frame = max(0, int(clock().Sub(s.TurnFrom)/busyFrame)%max(1, sp.frames))
	}
	img := caution.Image(spriteFrame(set, p.name, frame)).W(busyW).H(busyH)
	if sess := r.v.s; sess != nil && !p.still {
		urls := make([]string, sp.frames)
		for i := range urls {
			urls[i] = spriteFrame(set, p.name, i)
		}
		sess.Preload(urls...)
		anim := &uiSpriteAnim{s: sess, img: img, set: set, name: p.name, frames: sp.frames, frame: frame}
		anim.after(busyFrame)
	}
	return img
}

type uiSpriteAnim struct {
	s      *caution.Session
	img    *caution.Node
	set    Settings
	name   string
	frames int
	frame  int
}

func (an *uiSpriteAnim) after(d time.Duration) {
	time.AfterFunc(d, func() { an.s.Update(an.step) })
}

func (an *uiSpriteAnim) step() {
	select {
	case <-an.s.Done():
		return
	default:
	}
	if an.img.Parent() == nil {
		return
	}
	if !an.s.Visible() {
		an.after(busyHidden)
		return
	}
	an.frame = (an.frame + 1) % an.frames
	an.img.SetSrc(spriteFrame(an.set, an.name, an.frame))
	an.after(busyFrame)
}

func (r *uiRenderer) activity(a *Agent, s Snapshot) *caution.Node {
	text, color := statusLine(s)
	label := caution.Label(text).FontSize(12).Mono().Color(color).Clips().Fill(1)
	fx := glint{n: label}
	fx.set(r.v.set, text, 12, true, thinking(s))
	row := caution.HStack().Gap(busyPad).Align("center").H(30).Clips()
	if s.State == Working || s.State == Starting {
		row.Add(r.sprite(a, s))
	}
	row.Add(label)
	return row
}

func (r *uiRenderer) inbox(sc *uiscript.Scope) (*caution.Node, error) {
	list, _, _ := sc.Lookup("groups.need")
	rows, _ := uiscript.Items(list)
	byID := map[string]*Agent{}
	for _, a := range r.v.hub.Agents() {
		byID[a.Meta.ID] = a
	}
	box := caution.VStack().Gap(10).Align("stretch")
	if len(rows) == 0 {
		return box, nil
	}
	body := caution.VStack().Gap(8).Align("stretch")
	shown := 0
	for _, row := range rows {
		m, _ := row.(map[string]any)
		id, _ := m["id"].(string)
		a := byID[id]
		if a == nil {
			continue
		}
		shown++
		if shown > 5 {
			continue
		}
		t := newTile(r.v, a)
		t.update(a.Snapshot())
		if k, ok := uiscript.Number(m["key"]); ok {
			t.setBadge(int(k))
		}
		switch {
		case len(rows) > 3 && t.row != nil:
			body.Add(t.row)
		case t.card != nil:
			body.Add(t.card)
		default:
			body.Add(t.node)
		}
	}
	if shown > 5 {
		act := r.act
		body.Add(caution.Panel().H(40).Bg("$panel").Radius(10).Border("$edgeSoft", 1).Kids(
			caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { act.uiNextWaiting() }),
			caution.HStack().Gap(8).Align("center").Anchor(caution.A{Left: px(16), CenterY: px(0)}).Kids(
				caution.Label(fmt.Sprintf("%d more waiting", shown-5)).FontSize(13).Color("$inkDim"), kbd(platform.ModLabel+"J")),
		))
	}
	head := caution.HStack().Gap(8).Align("center").Kids(
		caution.Label("NEEDS YOU").FontSize(11).Weight(650).Color("$inkDim"),
		caution.Label(fmt.Sprint(shown)).FontSize(11).Weight(600).Color("$inkFaint"),
		caution.Panel().Fill(1),
		hints([][2]string{{"A  " + platform.ModLabel + "Y", "allow"}, {"D  " + platform.ShiftLabel + platform.ModLabel + "D", "deny"}, {platform.ShiftLabel + "A  " + platform.ShiftLabel + platform.ModLabel + "Y", "always"}, {platform.ModLabel + "J", "next"}, {"Enter", "open"}}),
	)
	return box.Kids(head, body), nil
}

func (r *uiRenderer) limits() *caution.Node {
	box := caution.HStack().Gap(24).Align("center")
	all := r.v.hub.Limits()
	for _, p := range providers {
		l, ok := all[p]
		if !ok || (!l.HasFive && !l.HasWeek) {
			continue
		}
		group := caution.HStack().Gap(10).Align("center").Kids(
			caution.Label(providerName(p)).FontSize(12).Color("$inkFaint"))
		if l.HasFive {
			group.Add(limitMeter("5h", "5-hour", l.Five, l.FiveResets))
		}
		if l.HasWeek {
			group.Add(limitMeter("7d", "Weekly", l.Week, l.WeekResets))
		}
		box.Add(group)
	}
	return box
}
