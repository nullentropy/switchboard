package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func uiToolsFixture(t *testing.T) (*uiWorld, *mcpServer) {
	t.Helper()
	w := uiRenderFixture(t)
	return w, &mcpServer{hub: w.h}
}

func uiSpec(t *testing.T, src string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(src), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

const uiHeartOfGold = `{
  "name": "Heart of Gold",
  "slots": {
    "board": {"type": "vstack", "props": {"gap": 12, "pad": 24}, "kids": [
      {"type": "sb.inbox"},
      {"type": "grid", "props": {"columns": 4, "gap": 10}, "each": "groups.work", "as": "w",
       "kids": [{"type": "sb.tile", "props": {"session": "{{w.id}}"}}]}
    ]},
    "railRow": {"type": "hstack", "props": {"gap": 6, "align": "center"},
      "onClick": {"action": "open_session", "session": "{{s.id}}"},
      "kids": [
        {"type": "sb.stateDot", "props": {"session": "{{s.id}}"}},
        {"type": "label", "text": "{{s.title}}", "props": {"color": "$ink"}},
        {"type": "label", "if": "s.cost > 1", "text": "${{s.cost}}", "props": {"color": "$inkFaint"}}
      ]}
  }
}`

func uiNamed(name string) string {
	return fmt.Sprintf(`{"name": %q, "slots": {"header": {"type": "label", "text": "Don't Panic: {{totals.need}} need you"}}}`, name)
}

func TestSaveUIStoresAndSwitchesToAValidInterface(t *testing.T) {
	w, m := uiToolsFixture(t)
	msg, err := m.uiCall("save_ui", map[string]any{"interface": uiSpec(t, uiHeartOfGold)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "Heart of Gold") {
		t.Fatalf("reply %q", msg)
	}
	name, raw := w.h.ActiveUI()
	if name != "Heart of Gold" {
		t.Fatalf("active %q", name)
	}
	if ui, errs := parseUI(raw); len(errs) > 0 || len(ui.Slots) != 2 {
		t.Fatalf("stored %s: %v", raw, errs)
	}

	set := newHub(map[string]string{}, w.h.path).Settings()
	if set.Interface != "Heart of Gold" || set.Interfaces["Heart of Gold"] == nil {
		t.Fatalf("after restart interface %q, saved %v", set.Interface, len(set.Interfaces))
	}

	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Heart of Gold")}); err != nil {
		t.Fatalf("replacing from a JSON string: %v", err)
	}
	_, raw = w.h.ActiveUI()
	if ui, _ := parseUI(raw); ui.Slots["header"] == nil || ui.Slots["board"] != nil {
		t.Fatalf("not replaced: %s", raw)
	}
	if n := len(w.h.Settings().Interfaces); n != 1 {
		t.Fatalf("%d interfaces after replacing", n)
	}
}

func TestSaveUIRefusesAnInvalidInterface(t *testing.T) {
	w, m := uiToolsFixture(t)
	bad := `{"name": "Vogon Poetry", "slots": {
	  "board": {"type": "vstack", "kids": [{"type": "towel"}, {"type": "button", "text": "Go", "onClick": {"action": "demolish_earth"}}]},
	  "railRow": {"type": "label", "text": "{{s.babelFish}}"}
	}}`
	_, err := m.uiCall("save_ui", map[string]any{"interface": uiSpec(t, bad)})
	if err == nil {
		t.Fatal("invalid interface saved")
	}
	for _, want := range []string{"towel", "demolish_earth", "babelFish"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}

	big := fmt.Sprintf(`{"name": "Hyperspace Bypass", "slots": {"header": {"type": "label", "text": %q}}}`, strings.Repeat("4", uiMaxBytes))
	if _, err := m.uiCall("save_ui", map[string]any{"interface": big}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversized interface: %v", err)
	}
	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Default")}); err == nil {
		t.Fatal("saved over Default")
	}
	if _, err := m.uiCall("save_ui", map[string]any{"interface": "{not json"}); err == nil {
		t.Fatal("broken JSON accepted")
	}
	if _, err := m.uiCall("save_ui", map[string]any{}); err == nil {
		t.Fatal("missing interface accepted")
	}

	for i := range uiMaxInterfaces {
		if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed(fmt.Sprintf("Planet %d", i))}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Magrathea")}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("21st interface: %v", err)
	}
	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Planet 3")}); err != nil {
		t.Fatalf("replacing at the limit: %v", err)
	}
	set := w.h.Settings()
	if _, ok := set.Interfaces["Vogon Poetry"]; ok || len(set.Interfaces) != uiMaxInterfaces || set.Interface != "Planet 3" {
		t.Fatalf("saved %d, on %q", len(set.Interfaces), set.Interface)
	}
}

func TestSaveUIRefusesAnInterfaceThatFailsTheDryRender(t *testing.T) {
	w, m := uiToolsFixture(t)
	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Earth")}); err != nil {
		t.Fatal(err)
	}
	missing := `{"name": "Deep Thought", "slots": {"board": {"type": "sb.stateDot", "props": {"session": "Deep Thought"}}}}`
	if _, errs := parseUI([]byte(missing)); len(errs) > 0 {
		t.Fatalf("fixture should pass the checks: %v", errs)
	}
	_, err := m.uiCall("save_ui", map[string]any{"interface": missing})
	if err == nil || !strings.Contains(err.Error(), "slots.board") || !strings.Contains(err.Error(), "Deep Thought") {
		t.Fatalf("dry render error: %v", err)
	}

	tooMany := `{"name": "Infinite Improbability", "slots": {"tile": {"type": "vstack", "each": "sessions", "as": "a", "kids": [
	  {"type": "vstack", "each": "sessions", "as": "b", "kids": [
	    {"type": "vstack", "each": "sessions", "as": "c", "kids": [
	      {"type": "vstack", "each": "sessions", "as": "d", "kids": [
	        {"type": "label", "each": "sessions", "as": "e", "text": "{{e.title}}"}]}]}]}]}}}`
	_, err = m.uiCall("save_ui", map[string]any{"interface": tooMany})
	if err == nil || !strings.Contains(err.Error(), "slots.tile") || !strings.Contains(err.Error(), fmt.Sprint(uiMaxNodes)) {
		t.Fatalf("node cap error: %v", err)
	}

	set := w.h.Settings()
	if set.Interface != "Earth" || len(set.Interfaces) != 1 {
		t.Fatalf("on %q with %d saved", set.Interface, len(set.Interfaces))
	}
}

func TestUseUISwitchesBetweenSavedInterfacesAndDefault(t *testing.T) {
	w, m := uiToolsFixture(t)
	for _, n := range []string{"Earth", "Magrathea"} {
		if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed(n)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.uiCall("use_ui", map[string]any{"name": "Earth"}); err != nil {
		t.Fatal(err)
	}
	if name, _ := w.h.ActiveUI(); name != "Earth" {
		t.Fatalf("active %q", name)
	}
	if _, err := m.uiCall("use_ui", map[string]any{"name": "Default"}); err != nil {
		t.Fatal(err)
	}
	if name, _ := w.h.ActiveUI(); name != "" || w.h.Settings().Interface != "" {
		t.Fatalf("active %q after Default", name)
	}
	if _, err := m.uiCall("use_ui", map[string]any{"name": "Milliways"}); err == nil {
		t.Fatal("switched to an unsaved interface")
	}
	if _, err := m.uiCall("use_ui", map[string]any{}); err == nil {
		t.Fatal("switched with no name")
	}

	if _, err := m.uiCall("use_ui", map[string]any{"name": "Magrathea"}); err != nil {
		t.Fatal(err)
	}
	w.h.bootUI(true)
	if name, _ := w.h.ActiveUI(); name != "" {
		t.Fatalf("shift start still shows %q", name)
	}
	if _, err := m.uiCall("use_ui", map[string]any{"name": "Magrathea"}); err != nil {
		t.Fatal(err)
	}
	if name, _ := w.h.ActiveUI(); name != "Magrathea" {
		t.Fatalf("use_ui during a shift start shows %q", name)
	}
}

func TestDeleteUIRemovesAnInterfaceAndFallsBackToDefault(t *testing.T) {
	w, m := uiToolsFixture(t)
	for _, n := range []string{"Earth", "Magrathea"} {
		if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed(n)}); err != nil {
			t.Fatal(err)
		}
	}
	msg, err := m.uiCall("delete_ui", map[string]any{"name": "Earth"})
	if err != nil {
		t.Fatal(err)
	}
	set := w.h.Settings()
	if _, ok := set.Interfaces["Earth"]; ok || set.Interface != "Magrathea" || strings.Contains(msg, "Default") {
		t.Fatalf("after deleting Earth: on %q, %d saved, %q", set.Interface, len(set.Interfaces), msg)
	}

	msg, err = m.uiCall("delete_ui", map[string]any{"name": "Magrathea"})
	if err != nil {
		t.Fatal(err)
	}
	set = w.h.Settings()
	if len(set.Interfaces) != 0 || set.Interface != "" || !strings.Contains(msg, "Default") {
		t.Fatalf("after deleting the one on: on %q, %d saved, %q", set.Interface, len(set.Interfaces), msg)
	}
	if name, _ := w.h.ActiveUI(); name != "" {
		t.Fatalf("active %q", name)
	}
	if _, err := m.uiCall("delete_ui", map[string]any{"name": "Magrathea"}); err == nil {
		t.Fatal("deleted a missing interface")
	}
	if _, err := m.uiCall("delete_ui", map[string]any{"name": "Default"}); err == nil {
		t.Fatal("deleted Default")
	}
}

func TestGetUIDescribesTheFormatAndReturnsSavedJSON(t *testing.T) {
	_, m := uiToolsFixture(t)
	out, err := m.uiCall("get_ui", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var empty map[string]any
	if err := json.Unmarshal([]byte(out), &empty); err != nil {
		t.Fatal(err)
	}
	if empty["on"] != "Default" || empty["saved"] != nil {
		t.Fatalf("with nothing saved: on %v, saved %v", empty["on"], empty["saved"])
	}

	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiSpec(t, uiHeartOfGold)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.uiCall("save_ui", map[string]any{"interface": uiNamed("Earth")}); err != nil {
		t.Fatal(err)
	}
	out, err = m.uiCall("get_ui", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		On    string `json:"on"`
		Saved []struct {
			Name  string   `json:"name"`
			On    bool     `json:"on"`
			Slots []string `json:"slots"`
		} `json:"saved"`
		Format struct {
			Types []string          `json:"types"`
			Props map[string]string `json:"props"`
		} `json:"format"`
		Slots []struct {
			Slot string `json:"slot"`
		} `json:"slots"`
		Data struct {
			Sessions []map[string]string `json:"sessions"`
			Groups   map[string]any      `json:"groups"`
		} `json:"data"`
		Actions []struct {
			Action string `json:"action"`
			Takes  string `json:"takes"`
		} `json:"actions"`
		Components []struct {
			Type  string   `json:"type"`
			Props []string `json:"props"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		t.Fatal(err)
	}
	if g.On != "Earth" || len(g.Saved) != 2 || g.Saved[0].Name != "Earth" || !g.Saved[0].On ||
		g.Saved[1].Name != "Heart of Gold" || g.Saved[1].On || !slices.Equal(g.Saved[1].Slots, []string{"board", "railRow"}) {
		t.Fatalf("saved list %+v, on %q", g.Saved, g.On)
	}
	if len(g.Slots) != 5 || !slices.Contains(g.Format.Types, "glass") || g.Format.Props["color"] == "" {
		t.Fatalf("format %+v, slots %+v", g.Format, g.Slots)
	}
	if len(g.Data.Sessions) != 1 || g.Data.Sessions[0]["cost"] != "dollars" || g.Data.Sessions[0]["title"] != "text" || len(g.Data.Groups) != 8 {
		t.Fatalf("data %+v", g.Data)
	}
	takes := map[string]string{}
	for _, a := range g.Actions {
		takes[a.Action] = a.Takes
	}
	if len(takes) != len(uiActions) || takes["open_session"] != "session" || takes["filter_goal"] != "goal" || takes["new_session"] != "" {
		t.Fatalf("actions %+v", g.Actions)
	}
	props := map[string][]string{}
	for _, c := range g.Components {
		props[c.Type] = c.Props
	}
	if len(props) != len(uiComponents) || !slices.Contains(props["sb.tile"], "session") || slices.Contains(props["sb.inbox"], "session") {
		t.Fatalf("components %+v", g.Components)
	}

	one, err := m.uiCall("get_ui", map[string]any{"name": "Heart of Gold"})
	if err != nil {
		t.Fatal(err)
	}
	ui, errs := parseUI([]byte(one))
	if len(errs) > 0 || ui.Name != "Heart of Gold" || ui.Slots["railRow"].OnClick.Action != "open_session" {
		t.Fatalf("get_ui by name: %s %v", one, errs)
	}
	if _, err := m.uiCall("get_ui", map[string]any{"name": "Milliways"}); err == nil {
		t.Fatal("got an unsaved interface")
	}
	if _, err := m.uiCall("get_ui", map[string]any{"name": "Default"}); err == nil {
		t.Fatal("got JSON for Default")
	}
}
