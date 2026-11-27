package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"
)

type uiCall struct {
	name, arg string
}

type uiFakeActor struct{ calls []uiCall }

func (f *uiFakeActor) rec(name, arg string) { f.calls = append(f.calls, uiCall{name, arg}) }

func (f *uiFakeActor) uiOpenSession(a *Agent) { f.rec("open_session", a.Meta.Title) }
func (f *uiFakeActor) uiNewSession()          { f.rec("new_session", "") }
func (f *uiFakeActor) uiFocusInput()          { f.rec("focus_input", "") }
func (f *uiFakeActor) uiOpenPalette()         { f.rec("open_palette", "") }
func (f *uiFakeActor) uiNextWaiting()         { f.rec("next_waiting", "") }
func (f *uiFakeActor) uiShowBoard()           { f.rec("show_board", "") }
func (f *uiFakeActor) uiOpenSettings()        { f.rec("open_settings", "") }
func (f *uiFakeActor) uiAnswer(a *Agent, allow, always bool) {
	f.rec(fmt.Sprintf("answer allow=%v always=%v", allow, always), a.Meta.Title)
}
func (f *uiFakeActor) uiInterrupt(a *Agent)       { f.rec("interrupt", a.Meta.Title) }
func (f *uiFakeActor) uiFilterGoal(goal string)   { f.rec("filter_goal", goal) }
func (f *uiFakeActor) uiClearFilter()             { f.rec("clear_filter", "") }
func (f *uiFakeActor) uiStopSession(a *Agent)     { f.rec("stop_session", a.Meta.Title) }
func (f *uiFakeActor) uiRemoveSession(a *Agent)   { f.rec("remove_session", a.Meta.Title) }
func (f *uiFakeActor) uiMarkGoalDone(goal string) { f.rec("mark_goal_done", goal) }

type uiWorld struct {
	h                              *Hub
	v                              *View
	act                            *uiFakeActor
	arthur, ford, zaphod, trillian *Agent
	marvin                         *Agent
}

func uiRenderFixture(t *testing.T) *uiWorld {
	t.Helper()
	h, now := uiFixture(t)
	w := &uiWorld{h: h, act: &uiFakeActor{}}
	w.arthur = wallSession(t, h, Meta{Title: "Arthur Dent", Provider: "claude", Cwd: "/earth", Goal: "Find the Question"})
	w.arthur.State, w.arthur.Cost, w.arthur.TurnFrom = NeedsYou, 4.2, now.Add(-90*time.Second)
	w.arthur.Pending = []*Pending{{ReqID: "tea", Tool: "Bash", Summary: "make tea", Input: map[string]any{"command": "make tea"}}}
	w.ford = wallSession(t, h, Meta{Title: "Ford Prefect", Provider: "codex", Cwd: "/betelgeuse", Goal: "Find the Question"})
	w.ford.State, w.ford.Cost, w.ford.TurnFrom, w.ford.Activity = Working, 0.5, now.Add(-42*time.Second), "Thinking"
	w.zaphod = wallSession(t, h, Meta{Title: "Zaphod Beeblebrox", Provider: "gemini", Cwd: "/heart-of-gold"})
	w.zaphod.State, w.zaphod.Cost, w.zaphod.TurnFrom, w.zaphod.Activity = Working, 12, now.Add(-5*time.Second), "Read improbability.go"
	w.trillian = wallSession(t, h, Meta{Title: "Trillian", Provider: "claude", Cwd: "/heart-of-gold"})
	w.trillian.State = Ready
	w.marvin = wallSession(t, h, Meta{Title: "Marvin", Provider: "claude", Cwd: "/heart-of-gold"})
	w.marvin.State, w.marvin.Err = Failed, "Brain the size of a planet"
	h.limits = map[string]Limits{
		"claude": {Five: 64, Week: 30, HasFive: true, HasWeek: true, FiveResets: now.Add(2 * time.Hour)},
		"codex":  {Week: 12, HasWeek: true},
	}
	w.v = &View{hub: h, set: h.Settings(), tiles: map[*Agent]*tile{}}
	return w
}

func (w *uiWorld) data() map[string]any { return uiData(w.h, uiViewState{Columns: 3, Width: 1400}) }

func (w *uiWorld) row(a *Agent) map[string]any {
	for _, r := range w.data()["sessions"].([]any) {
		if m := r.(map[string]any); m["id"] == a.Meta.ID {
			return m
		}
	}
	return nil
}

func uiParse(t *testing.T, src string) *uiInterface {
	t.Helper()
	ui, errs := parseUI([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return ui
}

func (w *uiWorld) render(t *testing.T, ui *uiInterface, slot string, session any) *caution.Node {
	t.Helper()
	n, err := renderUI(w.v, w.act, ui.Slots[slot], w.data(), session)
	if err != nil {
		t.Fatalf("render %s: %v", slot, err)
	}
	return n
}

func uiWalk(n *caution.Node, fn func(*caution.Node)) {
	fn(n)
	for _, k := range n.Children() {
		uiWalk(k, fn)
	}
}

func uiFindAll(n *caution.Node, pred func(*caution.Node) bool) []*caution.Node {
	var out []*caution.Node
	uiWalk(n, func(x *caution.Node) {
		if pred(x) {
			out = append(out, x)
		}
	})
	return out
}

func uiTexts(n *caution.Node) []string {
	var out []string
	uiWalk(n, func(x *caution.Node) {
		switch x.Type() {
		case wire.Label:
			if s, _ := x.Prop("text").(string); s != "" {
				out = append(out, s)
			}
		case wire.Button:
			out = append(out, x.Prop("label").(string))
		case wire.RichText:
			for _, sp := range x.Prop("spans").([]caution.Span) {
				out = append(out, sp.Text)
			}
		}
	})
	return out
}

func uiByText(t *testing.T, n *caution.Node, text string) *caution.Node {
	t.Helper()
	for _, x := range uiFindAll(n, func(x *caution.Node) bool {
		return x.Prop("text") == text || x.Prop("label") == text
	}) {
		return x
	}
	t.Fatalf("no node with text %q in %v", text, uiTexts(n))
	return nil
}

func uiHandler(n *caution.Node, name string) reflect.Value {
	f := reflect.ValueOf(n).Elem().FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func uiClick(t *testing.T, n *caution.Node) {
	t.Helper()
	if fn, _ := uiHandler(n, "onClick").Interface().(func()); fn != nil {
		fn()
		return
	}
	if fn, _ := uiHandler(n, "onPick").Interface().(func(x, y float64, target *caution.Node)); fn != nil {
		fn(1, 1, nil)
		return
	}
	for _, k := range n.Children() {
		if k.Type() == wire.Glass {
			if fn, _ := uiHandler(k, "onPick").Interface().(func(x, y float64, target *caution.Node)); fn != nil {
				fn(1, 1, nil)
				return
			}
		}
	}
	t.Fatalf("%s node has no click handler", n.Type())
}

func TestUIRenderTheDocExampleBoard(t *testing.T) {
	w := uiRenderFixture(t)
	board := w.render(t, uiParse(t, uiDocExample), "board", nil)

	if board.Type() != wire.VStack || board.Prop("spacing") != 12.0 || board.Prop("padding") != 24.0 {
		t.Fatalf("board root: %s spacing %v padding %v", board.Type(), board.Prop("spacing"), board.Prop("padding"))
	}
	kids := board.Children()
	if len(kids) != 3 {
		t.Fatalf("board has %d kids, want the inbox and one grid per working session", len(kids))
	}
	texts := uiTexts(kids[0])
	for _, want := range []string{"NEEDS YOU", "Arthur Dent", "Allow  A", "Deny  D"} {
		if !slices.Contains(texts, want) {
			t.Errorf("inbox lacks %q: %v", want, texts)
		}
	}
	for i, want := range []string{"Ford Prefect", "Zaphod Beeblebrox"} {
		grid := kids[i+1]
		if grid.Type() != wire.Grid || len(grid.Prop("columns").([]map[string]any)) != 4 || grid.Prop("colGap") != 10.0 {
			t.Fatalf("grid: %s columns %v colGap %v", grid.Type(), grid.Prop("columns"), grid.Prop("colGap"))
		}
		tiles := grid.Children()
		if len(tiles) != 1 || tiles[0].Prop("height") != tileH {
			t.Fatalf("grid %d holds %d tiles", i, len(tiles))
		}
		if got := uiTexts(tiles[0]); !slices.Contains(got, want) {
			t.Errorf("grid %d tile lacks %q: %v", i, want, got)
		}
	}
}

func TestUIRenderTheDocExampleRailRow(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, uiDocExample)

	zaphod := w.render(t, ui, "railRow", w.row(w.zaphod))
	if got := uiTexts(zaphod); !slices.Contains(got, "Zaphod Beeblebrox") || !slices.Contains(got, "$12.00") {
		t.Fatalf("Zaphod's row texts %v", got)
	}
	ford := w.render(t, ui, "railRow", w.row(w.ford))
	if got := uiTexts(ford); slices.Contains(got, "$0.50") {
		t.Fatalf("Ford's row shows a cost under $1: %v", got)
	}
	if ford.Type() != wire.Panel || ford.Prop("height") == nil || ford.Prop("height").(float64) <= 0 {
		t.Fatalf("a clickable hstack sits in a sized panel, got %s height %v", ford.Type(), ford.Prop("height"))
	}
	if dots := uiFindAll(ford, func(n *caution.Node) bool { return n.Prop("text") == stateIcon(Working) }); len(dots) != 1 {
		t.Fatalf("Ford's row has %d working state dots", len(dots))
	}

	uiClick(t, ford)
	if want := []uiCall{{"open_session", "Ford Prefect"}}; !reflect.DeepEqual(w.act.calls, want) {
		t.Fatalf("clicking Ford's row called %v", w.act.calls)
	}
}

func TestUIRenderEveryNodeTypeAndProp(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"header": {
	  "type": "vstack", "props": {"pad": 8, "gap": 4, "align": "stretch", "bg": "$panel", "width": 600},
	  "kids": [
	    {"type": "panel", "props": {"height": 40, "bg": "#336699", "border": "$accent", "borderWidth": 2, "radius": 6, "anchor": "top"},
	     "kids": [{"type": "label", "text": "Don't Panic", "props": {"fontSize": 18, "weight": 700, "mono": true, "color": "$stNeed", "anchor": "center", "tip": "{{totals.need}} need you"}}]},
	    {"type": "hstack", "props": {"fill": 1, "gap": "{{view.columns}}"}, "kids": [
	      {"type": "richtext", "text": "Spent ${{totals.spent}}"},
	      {"type": "button", "text": "New"},
	      {"type": "progress", "props": {"value": 0.42, "width": 80}},
	      {"type": "image", "props": {"src": "data:image/png;base64,QUJD", "width": 16, "height": 16}}
	    ]},
	    {"type": "grid", "props": {"columns": "{{view.columns}}", "gap": 6},
	     "kids": [{"type": "label", "each": "sessions", "as": "row", "text": "{{row.key}}. {{row.title}}"}]},
	    {"type": "scroll", "props": {"height": 120, "pad": 3, "gap": 2}, "kids": [{"type": "label", "text": "Mostly harmless"}]},
	    {"type": "glass", "props": {"height": 10}}
	  ]}}}`)
	root := w.render(t, ui, "header", nil)

	checks := []struct {
		n    *caution.Node
		prop string
		want any
	}{
		{root, "padding", 8.0}, {root, "spacing", 4.0}, {root, "align", "stretch"}, {root, "bg", "$panel"}, {root, "width", 600.0},
	}
	kids := root.Children()
	types := []wire.Type{}
	for _, k := range kids {
		types = append(types, k.Type())
	}
	if want := []wire.Type{wire.Panel, wire.HStack, wire.Grid, wire.Scroll, wire.Glass}; !reflect.DeepEqual(types, want) {
		t.Fatalf("kids %v, want %v", types, want)
	}
	panel, row, grid, scroll := kids[0], kids[1], kids[2], kids[3]
	label := panel.Children()[0]
	checks = append(checks, []struct {
		n    *caution.Node
		prop string
		want any
	}{
		{panel, "height", 40.0}, {panel, "bg", "#336699"}, {panel, "borderColor", "$accent"}, {panel, "borderWidth", 2.0}, {panel, "radius", 6.0},
		{label, "text", "Don't Panic"}, {label, "size", 18.0}, {label, "weight", 700}, {label, "mono", true}, {label, "color", "$stNeed"},
		{label, "tip", "1 need you"},
		{row, "spacing", 3.0},
		{grid, "colGap", 6.0}, {grid, "rowGap", 6.0},
		{kids[4], "height", 10.0},
	}...)
	for _, c := range checks {
		if got := c.n.Prop(c.prop); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s = %#v, want %#v", c.n.Type(), c.prop, got, c.want)
		}
	}
	if a := label.Prop("anchors").(map[string]any); a["centerX"] != 0.0 || a["centerY"] != 0.0 {
		t.Errorf("label anchor %v, want centred", a)
	}
	if a := panel.Prop("anchors").(map[string]any); a["left"] != 0.0 || a["right"] != 0.0 || a["top"] != 0.0 {
		t.Errorf("panel anchor %v, want pinned to the top", a)
	}
	if s, _ := row.Prop("stack").(map[string]any); s["kind"] != "fill" || s["weight"] != 1.0 {
		t.Error("hstack fill was not set")
	}

	rowKids := row.Children()
	if got := rowKids[0].Prop("spans").([]caution.Span)[0].Text; got != "Spent $16.70" {
		t.Errorf("richtext %q", got)
	}
	if rowKids[1].Prop("label") != "New" || rowKids[2].Prop("value") != 0.42 || rowKids[2].Prop("width") != 80.0 {
		t.Errorf("button %v progress %v", rowKids[1].Prop("label"), rowKids[2].Prop("value"))
	}
	if rowKids[3].Prop("src") != "data:image/png;base64,QUJD" || rowKids[3].Prop("height") != 16.0 {
		t.Errorf("image src %v", rowKids[3].Prop("src"))
	}

	if len(grid.Prop("columns").([]map[string]any)) != 3 {
		t.Errorf("grid columns bound to view.columns: %v", grid.Prop("columns"))
	}
	if got, want := uiTexts(grid), []string{"1. Arthur Dent", "2. Ford Prefect", "3. Zaphod Beeblebrox", "4. Marvin", "5. Trillian"}; !reflect.DeepEqual(got, want) {
		t.Errorf("grid rows %v, want %v", got, want)
	}

	inner := scroll.Children()[0]
	if inner.Type() != wire.VStack || inner.Prop("padding") != 3.0 || inner.Prop("spacing") != 2.0 || scroll.Prop("height") != 120.0 {
		t.Errorf("scroll holds %s padding %v spacing %v", inner.Type(), inner.Prop("padding"), inner.Prop("spacing"))
	}
	if got := uiTexts(inner); !reflect.DeepEqual(got, []string{"Mostly harmless"}) {
		t.Errorf("scroll content %v", got)
	}
}

func TestUIRenderTheBuiltInComponents(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"tile": {
	  "type": "vstack", "kids": [
	    {"type": "sb.tile", "props": {"session": "{{s.id}}", "height": 160}},
	    {"type": "sb.sprite", "props": {"session": "{{s.id}}"}},
	    {"type": "sb.activity", "props": {"session": "{{s.id}}", "width": 300}},
	    {"type": "sb.stateDot", "props": {"session": "{{s.id}}"}},
	    {"type": "sb.limits"}
	  ]}}}`)

	ford := w.render(t, ui, "tile", w.row(w.ford)).Children()
	tile, sprite, activity, dot, limits := ford[0], ford[1], ford[2], ford[3], ford[4]

	if tile.Prop("height") != 160.0 || !slices.Contains(uiTexts(tile), "Ford Prefect") {
		t.Errorf("tile height %v texts %v", tile.Prop("height"), uiTexts(tile))
	}
	if sprite.Type() != wire.Image || !strings.HasPrefix(sprite.Prop("src").(string), "data:image/") {
		t.Errorf("a working session's sprite is %s src %v", sprite.Type(), sprite.Prop("src"))
	}
	if activity.Prop("width") != 300.0 || !slices.Contains(uiTexts(activity), "Thinking") {
		t.Errorf("activity width %v texts %v", activity.Prop("width"), uiTexts(activity))
	}
	if len(uiFindAll(activity, func(n *caution.Node) bool { return n.Type() == wire.Image })) != 1 {
		t.Error("a working session's activity strip has no sprite")
	}
	if uiByText(t, dot, stateIcon(Working)).Prop("color") != "$stWork" {
		t.Errorf("Ford's state dot is not the working colour")
	}
	if got := uiTexts(limits); !slices.Contains(got, "Claude") || !slices.Contains(got, "Codex") || slices.Contains(got, "Gemini") {
		t.Errorf("limits texts %v", got)
	}

	trillian := w.render(t, ui, "tile", w.row(w.trillian)).Children()
	if trillian[1].Type() != wire.Panel || len(uiFindAll(trillian[2], func(n *caution.Node) bool { return n.Type() == wire.Image })) != 0 {
		t.Error("an idle session shows the busy sprite")
	}
	if uiByText(t, trillian[3], stateIcon(Ready)).Prop("color") != "$stReady" {
		t.Error("Trillian's state dot is not the ready colour")
	}
}

func TestUIRenderTheInboxCapsAtFiveAndSwitchesToRows(t *testing.T) {
	w := uiRenderFixture(t)
	for _, name := range []string{"Slartibartfast", "Fenchurch", "Eddie", "Deep Thought", "Prostetnic Vogon Jeltz"} {
		a := wallSession(t, w.h, Meta{Title: name, Provider: "claude", Cwd: "/magrathea"})
		a.State = NeedsYou
		a.Pending = []*Pending{{ReqID: name, Tool: "Edit", Summary: "fjords.go"}}
	}
	ui := uiParse(t, `{"name": "Guide", "slots": {"board": {"type": "sb.inbox"}}}`)
	inbox := w.render(t, ui, "board", nil)
	body := inbox.Children()[1]
	kids := body.Children()
	if len(kids) != 6 {
		t.Fatalf("inbox body has %d kids, want five sessions and the more row", len(kids))
	}
	for _, k := range kids[:5] {
		if k.Prop("height") != 52.0 {
			t.Errorf("with six waiting the inbox shows rows, got height %v", k.Prop("height"))
		}
	}
	if !slices.Contains(uiTexts(kids[5]), "1 more waiting") {
		t.Fatalf("more row texts %v", uiTexts(kids[5]))
	}
	uiClick(t, kids[5])
	if want := []uiCall{{"next_waiting", ""}}; !reflect.DeepEqual(w.act.calls, want) {
		t.Fatalf("the more row called %v", w.act.calls)
	}

	w.h.mu.Lock()
	w.h.agents = []*Agent{w.ford}
	w.h.mu.Unlock()
	if empty := w.render(t, ui, "board", nil); len(empty.Children()) != 0 {
		t.Fatalf("with nobody waiting the inbox has %d kids", len(empty.Children()))
	}
}

func TestUIRenderEveryActionReachesTheActor(t *testing.T) {
	cases := []struct {
		action, arg string
		want        uiCall
	}{
		{"open_session", `"session": "{{s.id}}"`, uiCall{"open_session", "Arthur Dent"}},
		{"new_session", "", uiCall{"new_session", ""}},
		{"focus_input", "", uiCall{"focus_input", ""}},
		{"open_palette", "", uiCall{"open_palette", ""}},
		{"next_waiting", "", uiCall{"next_waiting", ""}},
		{"show_board", "", uiCall{"show_board", ""}},
		{"open_settings", "", uiCall{"open_settings", ""}},
		{"allow", `"session": "{{s.id}}"`, uiCall{"answer allow=true always=false", "Arthur Dent"}},
		{"deny", `"session": "{{s.id}}"`, uiCall{"answer allow=false always=false", "Arthur Dent"}},
		{"always", `"session": "{{s.id}}"`, uiCall{"answer allow=true always=true", "Arthur Dent"}},
		{"interrupt", `"session": "{{s.id}}"`, uiCall{"interrupt", "Arthur Dent"}},
		{"filter_goal", `"goal": "{{s.goal}}"`, uiCall{"filter_goal", "Find the Question"}},
		{"clear_filter", "", uiCall{"clear_filter", ""}},
		{"stop_session", `"session": "{{s.id}}"`, uiCall{"stop_session", "Arthur Dent"}},
		{"remove_session", `"session": "{{s.id}}"`, uiCall{"remove_session", "Arthur Dent"}},
		{"mark_goal_done", `"goal": "{{s.goal}}"`, uiCall{"mark_goal_done", "Find the Question"}},
	}
	if len(cases) != len(uiActions) {
		t.Fatalf("%d cases for %d actions", len(cases), len(uiActions))
	}
	for _, typ := range []string{"button", "label", "panel", "glass", "hstack"} {
		for _, c := range cases {
			t.Run(typ+"/"+c.action, func(t *testing.T) {
				w := uiRenderFixture(t)
				click := `{"action": "` + c.action + `"`
				if c.arg != "" {
					click += ", " + c.arg
				}
				text := ""
				if uiTextTypes[typ] {
					text = `"text": "Go", `
				}
				ui := uiParse(t, `{"name": "Guide", "slots": {"tile": {"type": "`+typ+`", `+text+`"props": {"height": 20}, "onClick": `+click+`}}}}`)
				uiClick(t, w.render(t, ui, "tile", w.row(w.arthur)))
				if want := []uiCall{c.want}; !reflect.DeepEqual(w.act.calls, want) {
					t.Fatalf("got %v, want %v", w.act.calls, want)
				}
			})
		}
	}
}

func TestUIRenderASessionActionForAGoneSessionDoesNothing(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"tile": {"type": "button", "text": "Allow", "onClick": {"action": "allow", "session": "{{s.id}}"}}}}`)
	btn := w.render(t, ui, "tile", w.row(w.arthur))
	w.h.mu.Lock()
	w.h.agents = []*Agent{w.ford}
	w.h.mu.Unlock()
	uiClick(t, btn)
	if len(w.act.calls) != 0 {
		t.Fatalf("clicking for a removed session called %v", w.act.calls)
	}
}

func TestUIRenderCapsTheSlotAt2000Nodes(t *testing.T) {
	ui := uiParse(t, `{"name": "Guide", "slots": {"board": {
	  "type": "vstack", "kids": [{"type": "hstack", "each": "sessions", "as": "a",
	    "kids": [{"type": "label", "each": "sessions", "as": "b", "text": "{{a.title}} meets {{b.title}}"}]}]}}}`)
	for _, c := range []struct {
		sessions int
		ok       bool
	}{{44, true}, {45, false}} {
		w := uiRenderFixture(t)
		w.h.mu.Lock()
		w.h.agents = nil
		w.h.mu.Unlock()
		for i := range c.sessions {
			uiSet(wallSession(t, w.h, Meta{Title: fmt.Sprintf("Golgafrinchan %d", i), Cwd: "/ark-b"}), Ready, 0)
		}
		n, err := renderUI(w.v, w.act, ui.Slots["board"], w.data(), nil)
		if c.ok {
			if err != nil {
				t.Fatalf("%d sessions: %v", c.sessions, err)
			}
			total := 0
			uiWalk(n, func(*caution.Node) { total++ })
			if total != 1+c.sessions+c.sessions*c.sessions {
				t.Fatalf("%d sessions rendered %d nodes", c.sessions, total)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "more than 2000 nodes") {
			t.Fatalf("%d sessions: err %v, want the node cap", c.sessions, err)
		}
	}
}

func TestUIRenderTheCapCountsComponentNodes(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"board": {"type": "vstack", "kids": [
	  {"type": "vstack", "each": "sessions", "as": "a", "kids": [
	    {"type": "vstack", "each": "sessions", "as": "b", "kids": [
	      {"type": "vstack", "each": "sessions", "as": "c", "kids": [{"type": "sb.tile", "props": {"session": "{{c.id}}"}}]}]}]}]}}}`)
	_, err := renderUI(w.v, w.act, ui.Slots["board"], w.data(), nil)
	if err == nil || !strings.Contains(err.Error(), "more than 2000 nodes") {
		t.Fatalf("125 tiles: err %v, want the node cap", err)
	}
}

func TestUIRenderRecoversAPanicAsAnError(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"header": {"type": "hstack", "kids": [{"type": "sb.limits"}]}}}`)
	n, err := renderUI(nil, w.act, ui.Slots["header"], w.data(), nil)
	if err == nil || n != nil || !strings.Contains(err.Error(), "failed while rendering") {
		t.Fatalf("render with no view: node %v err %v", n, err)
	}
}

func TestUIRenderErrors(t *testing.T) {
	w := uiRenderFixture(t)
	cases := []struct {
		name string
		node *uiNode
		want string
	}{
		{"unknown type", &uiNode{Type: "terminal"}, `unknown node type "terminal"`},
		{"unknown session", &uiNode{Type: "sb.tile", Props: map[string]any{"session": "Agrajag"}}, `no session "Agrajag"`},
		{"kids on a label", &uiNode{Type: "label", Kids: []*uiNode{{Type: "label"}}}, "label cannot hold kids"},
		{"bad colour", &uiNode{Type: "panel", Props: map[string]any{"bg": "red"}}, "not a theme token"},
		{"compare text with <", &uiNode{Type: "label", If: "s.title < 3"}, "compares numbers only"},
		{"empty slot", nil, "the slot is empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := renderUI(w.v, w.act, c.node, w.data(), w.row(w.arthur))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err %v, want %q", err, c.want)
			}
		})
	}
}

func TestUIRenderASlotWhoseRootIsFilteredOutIsEmpty(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {
	  "railRow": {"type": "label", "if": "s.cost > 100", "text": "{{s.title}}"},
	  "board": {"type": "label", "each": "groups.need", "as": "n", "text": "{{n.title}}"}}}`)
	if n := w.render(t, ui, "railRow", w.row(w.arthur)); n.Type() != wire.Panel || len(n.Children()) != 0 {
		t.Fatalf("a root whose if fails renders %s with %d kids", n.Type(), len(n.Children()))
	}
	wallSession(t, w.h, Meta{Title: "Slartibartfast", Cwd: "/magrathea"}).State = NeedsYou
	if got := uiTexts(w.render(t, ui, "board", nil)); !reflect.DeepEqual(got, []string{"Arthur Dent", "Slartibartfast"}) {
		t.Fatalf("a repeated root renders %v", got)
	}
}

func TestUIRenderAClickableStackKeepsItsSizeOnTheWrapper(t *testing.T) {
	w := uiRenderFixture(t)
	ui := uiParse(t, `{"name": "Guide", "slots": {"tile": {
	  "type": "vstack", "props": {"width": 200, "fill": 2, "anchor": "top", "pad": 4, "gap": 2},
	  "onClick": {"action": "open_session", "session": "{{s.id}}"},
	  "kids": [{"type": "label", "text": "{{s.title}}", "props": {"fontSize": 16}}, {"type": "button", "text": "Allow", "onClick": {"action": "allow", "session": "{{s.id}}"}}]}}}`)
	wrap := w.render(t, ui, "tile", w.row(w.arthur))
	if wrap.Type() != wire.Panel || wrap.Prop("width") != 200.0 || wrap.Prop("height") != 20.0+2+26+8 {
		t.Fatalf("wrapper %s width %v height %v", wrap.Type(), wrap.Prop("width"), wrap.Prop("height"))
	}
	if s, _ := wrap.Prop("stack").(map[string]any); s["weight"] != 2.0 {
		t.Fatalf("wrapper fill %v", wrap.Prop("stack"))
	}
	if a, _ := wrap.Prop("anchors").(map[string]any); a["top"] != 0.0 || a["bottom"] != nil {
		t.Fatalf("wrapper anchors %v", wrap.Prop("anchors"))
	}
	kids := wrap.Children()
	if len(kids) != 2 || kids[0].Type() != wire.Glass || kids[1].Type() != wire.VStack {
		t.Fatalf("wrapper holds the glass under the stack, got %d kids", len(kids))
	}
	uiClick(t, uiByText(t, wrap, "Allow"))
	uiClick(t, wrap)
	want := []uiCall{{"answer allow=true always=false", "Arthur Dent"}, {"open_session", "Arthur Dent"}}
	if !reflect.DeepEqual(w.act.calls, want) {
		t.Fatalf("calls %v, want %v", w.act.calls, want)
	}
}
