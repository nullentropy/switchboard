package uiscript

import (
	"fmt"
	"strings"
	"testing"
)

const docExample = `{
  "name": "Compact wall",
  "slots": {
    "board": {
      "type": "vstack", "props": {"gap": 12, "pad": 24},
      "kids": [
        {"type": "sb.inbox"},
        {"type": "grid", "props": {"columns": 4, "gap": 10},
         "each": "groups.work", "as": "s",
         "kids": [{"type": "sb.tile", "props": {"session": "{{s.id}}"}}]}
      ]
    },
    "railRow": {
      "type": "hstack", "props": {"gap": 6, "align": "center"},
      "onClick": {"action": "open_session", "session": "{{s.id}}"},
      "kids": [
        {"type": "sb.stateDot", "props": {"session": "{{s.id}}"}},
        {"type": "label", "text": "{{s.title}}", "props": {"color": "$ink"}},
        {"type": "label", "if": "s.cost > 1", "text": "${{s.cost}}", "props": {"color": "$inkFaint"}}
      ]
    }
  }
}`

func TestParseUIDocExample(t *testing.T) {
	ui, errs := Parse([]byte(docExample))
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if ui.Name != "Compact wall" {
		t.Fatalf("name %q", ui.Name)
	}
	grid := ui.Slots["board"].Kids[1]
	if grid.Type != "grid" || grid.Each != "groups.work" || grid.As != "s" || grid.Kids[0].Props["session"] != "{{s.id}}" {
		t.Fatalf("grid parsed wrong: %+v", grid)
	}
	row := ui.Slots["railRow"]
	if row.OnClick == nil || row.OnClick.Action != "open_session" || row.OnClick.Session != "{{s.id}}" {
		t.Fatalf("onClick parsed wrong: %+v", row.OnClick)
	}
	if row.Kids[2].If != "s.cost > 1" || row.Kids[2].Text != "${{s.cost}}" {
		t.Fatalf("label parsed wrong: %+v", row.Kids[2])
	}
}

func uiWith(slot, node string) string {
	return fmt.Sprintf(`{"name":"T","slots":{%q:%s}}`, slot, node)
}

func TestParseUIRefusals(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"bad json", `{"name":`, "invalid interface JSON"},
		{"unknown top key", `{"name":"T","slots":{},"x":1}`, `unknown field "x"`},
		{"unknown node key", uiWith("board", `{"type":"label","colour":"$ink"}`), `unknown field "colour"`},
		{"no name", `{"slots":{}}`, "needs a name"},
		{"reserved name", `{"name":"default","slots":{}}`, "reserved"},
		{"unknown slot", uiWith("footer", `{"type":"label"}`), "unknown slot"},
		{"unknown type", uiWith("board", `{"type":"textinput"}`), `unknown node type "textinput"`},
		{"shader type", uiWith("board", `{"type":"shader"}`), `unknown node type "shader"`},
		{"unknown component", uiWith("board", `{"type":"sb.terminal"}`), `unknown node type "sb.terminal"`},
		{"unknown prop", uiWith("board", `{"type":"label","props":{"blink":true}}`), `label has no prop "blink"`},
		{"type prop elsewhere", uiWith("board", `{"type":"vstack","props":{"columns":2}}`), `vstack has no prop "columns"`},
		{"style on component", uiWith("board", `{"type":"sb.inbox","props":{"color":"$ink"}}`), `sb.inbox has no prop "color"`},
		{"session on inbox", uiWith("board", `{"type":"sb.inbox","props":{"session":"x"}}`), `sb.inbox has no prop "session"`},
		{"component without session", uiWith("tile", `{"type":"sb.sprite"}`), "sb.sprite needs a session"},
		{"bad number", uiWith("board", `{"type":"vstack","props":{"gap":"wide"}}`), "must be a number or a single {{path}}"},
		{"negative number", uiWith("board", `{"type":"vstack","props":{"gap":-1}}`), "0 or more"},
		{"bad columns", uiWith("board", `{"type":"grid","props":{"columns":2.5}}`), "whole number"},
		{"bad progress", uiWith("board", `{"type":"progress","props":{"value":2}}`), "0 to 1"},
		{"bad bool", uiWith("board", `{"type":"label","props":{"mono":"yes"}}`), "true or false"},
		{"bad colour", uiWith("board", `{"type":"label","props":{"color":"red"}}`), "theme token"},
		{"bad align", uiWith("board", `{"type":"vstack","props":{"align":"middle"}}`), "start, center, end or stretch"},
		{"bad anchor", uiWith("board", `{"type":"vstack","props":{"anchor":"middle"}}`), "fill, center"},
		{"image url", uiWith("board", `{"type":"image","props":{"src":"https://x/y.png"}}`), "data:image/"},
		{"num prop bound to text", uiWith("tile", `{"type":"vstack","props":{"gap":"{{s.title}}"}}`), "s.title is not a number"},
		{"text on stack", uiWith("board", `{"type":"vstack","text":"hi"}`), "vstack has no text"},
		{"kids on label", uiWith("board", `{"type":"label","kids":[{"type":"label"}]}`), "label cannot hold kids"},
		{"unknown path", uiWith("board", `{"type":"label","text":"{{totals.cost}}"}`), "unknown path totals.cost"},
		{"s outside session slot", uiWith("board", `{"type":"label","text":"{{s.title}}"}`), "unknown path s.title"},
		{"old limits field", uiWith("board", `{"type":"label","text":"{{limits.text}}"}`), "unknown path limits.text"},
		{"limit field outside each", uiWith("board", `{"type":"label","text":"{{limits.five}}"}`), "unknown path limits.five"},
		{"limit text compared as number", uiWith("board", `{"type":"vstack","each":"limits","as":"l","if":"l.fiveResets > 1"}`), "l.fiveResets is not a number"},
		{"list in text", uiWith("board", `{"type":"label","text":"{{sessions}}"}`), "cannot go in text"},
		{"unclosed binding", uiWith("board", `{"type":"label","text":"{{totals.need"}`), "unclosed"},
		{"bad binding", uiWith("board", `{"type":"label","text":"{{totals need}}"}`), "not a path"},
		{"each not list", uiWith("board", `{"type":"vstack","each":"totals","as":"t"}`), "totals is not a list"},
		{"each unknown", uiWith("board", `{"type":"vstack","each":"groups.lost","as":"t"}`), "unknown path groups.lost"},
		{"each without as", uiWith("board", `{"type":"vstack","each":"sessions"}`), "each needs as"},
		{"as without each", uiWith("board", `{"type":"vstack","as":"x"}`), "as needs each"},
		{"as shadows data", uiWith("board", `{"type":"vstack","each":"sessions","as":"goals"}`), "already a data name"},
		{"as shadows s", uiWith("tile", `{"type":"vstack","each":"sessions","as":"s"}`), "already a data name"},
		{"as not a name", uiWith("board", `{"type":"vstack","each":"sessions","as":"a.b"}`), "not a plain name"},
		{"if unknown path", uiWith("board", `{"type":"label","if":"totals.cost > 1"}`), "unknown path totals.cost"},
		{"if bad syntax", uiWith("board", `{"type":"label","if":"totals.need >"}`), "ends early"},
		{"if bad joiner", uiWith("board", `{"type":"label","if":"totals.need totals.work"}`), "expected && or ||"},
		{"if string lt", uiWith("tile", `{"type":"label","if":"s.state < \"b\""}`), "only compares numbers"},
		{"if number on text", uiWith("tile", `{"type":"label","if":"s.title == 3"}`), "s.title is not a number"},
		{"if text on number", uiWith("tile", `{"type":"label","if":"s.cost == 'x'"}`), "s.cost is not text"},
		{"if not compare", uiWith("tile", `{"type":"label","if":"!s.cost > 1"}`), "! cannot go before"},
		{"if bare word", uiWith("tile", `{"type":"label","if":"s.state == Working"}`), "compare with a number or a quoted string"},
		{"unknown action", uiWith("board", `{"type":"button","onClick":{"action":"run_command"}}`), `unknown action "run_command"`},
		{"unknown action key", uiWith("board", `{"type":"button","onClick":{"action":"allow","cmd":"x"}}`), `unknown field "cmd"`},
		{"action needs session", uiWith("board", `{"type":"button","onClick":{"action":"allow"}}`), "allow needs session"},
		{"action needs goal", uiWith("board", `{"type":"button","onClick":{"action":"filter_goal"}}`), "filter_goal needs goal"},
		{"action takes nothing", uiWith("board", `{"type":"button","onClick":{"action":"open_palette","session":"x"}}`), "open_palette takes no session"},
		{"action wrong arg", uiWith("board", `{"type":"button","onClick":{"action":"open_session","session":"x","goal":"g"}}`), "open_session takes no goal"},
		{"action bad path", uiWith("board", `{"type":"button","onClick":{"action":"open_session","session":"{{s.id}}"}}`), "unknown path s.id"},
		{"too big", `{"name":"T","slots":{},"pad":"` + strings.Repeat("x", MaxBytes) + `"}`, "the limit is 65536"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, errs := Parse([]byte(c.src))
			if !strings.Contains(strings.Join(errs, "\n"), c.want) {
				t.Fatalf("want an error containing %q, got %v", c.want, errs)
			}
		})
	}
}

func TestParseUIReportsLocationAndAllErrors(t *testing.T) {
	_, errs := Parse([]byte(uiWith("board", `{"type":"vstack","kids":[{"type":"label","props":{"gap":"x"}},{"type":"nope"}]}`)))
	if len(errs) != 2 {
		t.Fatalf("want 2 errors, got %v", errs)
	}
	if !strings.HasPrefix(errs[0], "slots.board.kids[0].props.gap: ") || !strings.HasPrefix(errs[1], "slots.board.kids[1].type: ") {
		t.Fatalf("locations wrong: %v", errs)
	}
}

func TestParseUIAccepts(t *testing.T) {
	for _, src := range []string{
		uiWith("tile", `{"type":"vstack","props":{"gap":"{{view.columns}}","mono":true,"bg":"#12abEF","fill":1,"anchor":"fill","tip":"{{s.title}} in {{s.folder}}"}}`),
		uiWith("tile", `{"type":"label","props":{"mono":"{{s.selected}}"}}`),
		uiWith("board", `{"type":"vstack","each":"limits","as":"l","kids":[{"type":"label","if":"l.hasFive && l.five > 80","text":"{{l.name}} {{l.five}}% {{l.fiveResets}}"},{"type":"progress","props":{"value":"{{l.week}}"}},{"type":"label","if":"l.provider == 'codex' || !l.hasWeek","text":"{{l.weekResets}}"}]}`),
		uiWith("board", `{"type":"image","props":{"src":"data:image/png;base64,AAAA","width":20}}`),
		uiWith("board", `{"type":"vstack","each":"goals","as":"g","kids":[{"type":"vstack","each":"g.sessions","as":"t","if":"g.done && t.state == 'Working' || !t.goalDone","kids":[{"type":"sb.activity","props":{"session":"{{t.id}}","height":12}}]}]}`),
		uiWith("header", `{"type":"button","text":"New","onClick":{"action":"new_session"}}`),
		uiWith("header", `{"type":"button","if":"view.goal","onClick":{"action":"filter_goal","goal":"{{view.goal}}"}}`),
		uiWith("empty", `{"type":"label","if":"totals.spent > 0.5 || load.overloaded","text":"{{totals.spent}} {{update.version}}"}`),
		uiWith("railRow", `{"type":"label","if":"s.key != 0","text":"{{s.key}}"}`),
		uiWith("board", `{"type":"sb.limits","props":{"width":200}}`),
	} {
		if _, errs := Parse([]byte(src)); len(errs) > 0 {
			t.Errorf("%s: %v", src, errs)
		}
	}
}

func TestCheckUIName(t *testing.T) {
	saved := map[string]string{}
	for i := 0; i < MaxInterfaces; i++ {
		saved[fmt.Sprintf("ui%d", i)] = "{}"
	}
	if err := CheckName("ui3", saved); err != nil {
		t.Fatalf("replacing an existing name: %v", err)
	}
	if err := CheckName("new", saved); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("21st interface: %v", err)
	}
	if err := CheckName("Default", nil); err == nil {
		t.Fatal("Default accepted")
	}
	if err := CheckName(" ", nil); err == nil {
		t.Fatal("blank name accepted")
	}
	if err := CheckName("Mine", nil); err != nil {
		t.Fatal(err)
	}
}

func uiTestData() map[string]any {
	a := map[string]any{"id": "a1", "title": "Alpha", "state": "Working", "cost": 1.5, "elapsed": 125, "selected": true, "key": 1, "goalDone": false}
	b := map[string]any{"id": "b2", "title": "Beta", "state": "Ready", "cost": 0.25, "elapsed": 3, "selected": false, "goalDone": true}
	return map[string]any{
		"sessions": []map[string]any{a, b},
		"groups":   map[string]any{"work": []any{a}, "idle": []any{b}, "need": []any{}},
		"goals":    []any{map[string]any{"name": "ship", "done": false, "sessions": []any{a, b}}},
		"totals":   map[string]any{"need": 0, "work": 1, "done": 0, "spent": 1.75},
		"limits": []any{
			map[string]any{"provider": "claude", "name": "Claude", "five": 12.0, "week": 85.5, "hasFive": true, "hasWeek": true, "fiveResets": "15:04", "weekResets": "Sat 05:00"},
			map[string]any{"provider": "codex", "name": "Codex", "five": 0.0, "week": 3.0, "hasFive": false, "hasWeek": true, "fiveResets": "", "weekResets": ""},
		},
		"view": map[string]any{"columns": 4, "width": 1280.5, "goal": "", "project": "sb"},
	}
}

func TestUIBindings(t *testing.T) {
	data := uiTestData()
	sc := NewScope(data, data["sessions"].([]map[string]any)[1])
	cases := map[string]string{
		"{{s.title}}":                      "Beta",
		"${{s.cost}}":                      "$0.25",
		"{{totals.spent}} spent":           "1.75 spent",
		"{{ view.columns }} cols":          "4 cols",
		"{{view.width}}":                   "1280.5",
		"{{s.elapsed}}s":                   "3s",
		"{{s.selected}}/{{s.goalDone}}":    "false/true",
		"[{{s.key}}]":                      "[]",
		"[{{limits.text}}]":                "[]",
		"plain":                            "plain",
		"{{s.id}}-{{s.state}}-{{s.title}}": "b2-Ready-Beta",
	}
	for in, want := range cases {
		got, err := sc.Interp(in)
		if err != nil || got != want {
			t.Errorf("interp(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := sc.Interp("{{s.id"); err == nil {
		t.Error("unclosed binding rendered")
	}
	if _, err := NewScope(map[string]any{"groups": map[string]any{}}, nil).Interp("{{groups}}"); err == nil {
		t.Error("a group printed as text")
	}
}

func TestUIValueKeepsTypes(t *testing.T) {
	data := uiTestData()
	sc := NewScope(data, data["sessions"].([]map[string]any)[0])
	for in, want := range map[any]any{
		"{{view.columns}}": 4,
		"{{s.selected}}":   true,
		"{{s.cost}}":       1.5,
		"id {{s.id}}":      "id a1",
		12.0:               12.0,
	} {
		got, err := sc.Value(in)
		if err != nil || got != want {
			t.Errorf("value(%v) = %v (%T), %v; want %v", in, got, got, err, want)
		}
	}
	if got, err := sc.Value("{{update.ready}}"); err != nil || got != nil {
		t.Errorf("missing value = %v, %v", got, err)
	}
	a, err := sc.Action(&Action{Action: "open_session", Session: "{{s.id}}"})
	if err != nil || a != (Action{Action: "open_session", Session: "a1"}) {
		t.Errorf("action = %+v, %v", a, err)
	}
}

func TestUIConditions(t *testing.T) {
	data := uiTestData()
	sc := NewScope(data, data["sessions"].([]map[string]any)[0])
	cases := map[string]bool{
		"s.selected":                            true,
		"!s.selected":                           false,
		"s.goalDone":                            false,
		"!s.key":                                false,
		"s.title":                               true,
		"view.goal":                             false,
		"view.project":                          true,
		"groups.work":                           true,
		"groups.need":                           false,
		"groups.done":                           false,
		"update.ready":                          false,
		"!update.ready":                         true,
		"s.cost > 1":                            true,
		"s.cost < 1":                            false,
		"s.elapsed > 60":                        true,
		"s.key == 1":                            true,
		"s.key != 1":                            false,
		"s.state == \"Working\"":                true,
		"s.state == 'Working'":                  true,
		"s.state != 'Working'":                  false,
		"s.title == 'a b'":                      false,
		"s.key > 0 && s.state == 'Ready'":       false,
		"s.key > 0 || s.state == 'Ready'":       true,
		"view.goal || s.cost > 1 && s.key == 2": false,
		"s.cost > 1 && s.key == 1 || view.goal": true,
		"load.load > 1":                         false,
		"load.load == 1":                        false,
		"load.load != 1":                        true,
	}
	for in, want := range cases {
		got, err := sc.cond(in)
		if err != nil || got != want {
			t.Errorf("cond(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := sc.cond("s.title > 1"); err == nil {
		t.Error("< on text did not error")
	}
	if _, err := sc.cond("s.cost >"); err == nil {
		t.Error("bad syntax did not error")
	}
}

func TestUIExpandEachAndIf(t *testing.T) {
	data := uiTestData()
	root := NewScope(data, nil)
	titles := func(scs []*Scope, err error) string {
		if err != nil {
			return "error: " + err.Error()
		}
		var out []string
		for _, s := range scs {
			v, _ := s.Interp("{{x.title}}")
			out = append(out, v)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		node Node
		want string
	}{
		{Node{Each: "sessions", As: "x"}, "Alpha,Beta"},
		{Node{Each: "groups.work", As: "x"}, "Alpha"},
		{Node{Each: "groups.need", As: "x"}, ""},
		{Node{Each: "groups.fail", As: "x"}, ""},
		{Node{Each: "sessions", As: "x", If: "x.cost < 1"}, "Beta"},
		{Node{Each: "sessions", As: "x", If: "x.state == 'Done'"}, ""},
		{Node{Each: "view", As: "x"}, "error: view is not a list"},
	}
	for _, c := range cases {
		if got := titles(root.Expand(&c.node)); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.node, got, c.want)
		}
	}
	if scs, err := root.Expand(&Node{If: "totals.work > 0"}); err != nil || len(scs) != 1 || scs[0] != root {
		t.Errorf("if without each kept %v, %v", scs, err)
	}
	if scs, err := root.Expand(&Node{If: "totals.work > 5"}); err != nil || len(scs) != 0 {
		t.Errorf("false if kept %v, %v", scs, err)
	}

	lims, err := root.Expand(&Node{Each: "limits", As: "l", If: "l.hasWeek && l.week > 50"})
	if err != nil || len(lims) != 1 {
		t.Fatalf("each over limits: %d, %v", len(lims), err)
	}
	if got, _ := lims[0].Interp("{{l.name}} {{l.five}}/{{l.week}} {{l.weekResets}}"); got != "Claude 12/85.5 Sat 05:00" {
		t.Fatalf("limit entry: %q", got)
	}

	goals, _ := root.Expand(&Node{Each: "goals", As: "g"})
	if len(goals) != 1 {
		t.Fatalf("goals: %d", len(goals))
	}
	inner, err := goals[0].Expand(&Node{Each: "g.sessions", As: "t", If: "t.goalDone || g.done"})
	if err != nil || len(inner) != 1 {
		t.Fatalf("nested each: %d, %v", len(inner), err)
	}
	if got, _ := inner[0].Interp("{{g.name}}:{{t.title}}:{{t.cost}}"); got != "ship:Beta:0.25" {
		t.Fatalf("nested scope: %q", got)
	}
}
