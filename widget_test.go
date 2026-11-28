package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"switchboard/widget"
)

func widgetArgs(t *testing.T, js string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAWidgetIsReplacedByIdAndLocksOnceAnswered(t *testing.T) {
	a, d := stubAgent(t)
	res, err := a.ShowWidget(widgetArgs(t, `{"id":"ship","kind":"buttons","buttons":{"buttons":[{"label":"Ship it"},{"label":"Hold"}]}}`))
	if err != nil || res != "shown ship" {
		t.Fatalf("show gave %q, %v", res, err)
	}
	res, err = a.ShowWidget(widgetArgs(t, `{"id":"ship","kind":"buttons","title":"Release 42","buttons":{"buttons":[{"label":"Ship it"}]}}`))
	if err != nil || res != "updated ship" {
		t.Fatalf("replace gave %q, %v", res, err)
	}
	s := a.Snapshot()
	if len(s.Entries) != 1 || s.Entries[0].Widget.Title != "Release 42" {
		t.Fatalf("replacing made %d entries", len(s.Entries))
	}

	a.AnswerWidget("ship", widget.Answer{Clicked: "Ship it"})
	a.AnswerWidget("ship", widget.Answer{Clicked: "Ship it"})
	s = a.Snapshot()
	if got := d.delivered(); len(got) != 1 || got[0] != `<switchboard-widget id="ship" kind="buttons">{"clicked":"Ship it"}</switchboard-widget>` {
		t.Fatalf("the agent was sent %q", got)
	}
	if u := userTexts(s); len(u) != 0 {
		t.Fatalf("the click showed as a You row: %q", u)
	}
	if s.Entries[0].Answer == nil || s.Entries[0].Answer.Clicked != "Ship it" {
		t.Fatal("the widget did not record the click")
	}
	if len(s.Meta.Checkpoints) != 0 {
		t.Fatalf("a click took %d checkpoints", len(s.Meta.Checkpoints))
	}
	if _, err := a.ShowWidget(widgetArgs(t, `{"id":"ship","kind":"progress","progress":{"bars":[{"label":"a","value":0.5}]}}`)); err == nil || !strings.Contains(err.Error(), "answered") {
		t.Fatalf("replacing an answered widget gave %v", err)
	}
}

func TestAConversationHoldsAtMostFiftyWidgets(t *testing.T) {
	a, _ := stubAgent(t)
	kv := `{"kind":"keyvalue","keyvalue":{"items":[{"key":"answer","value":"42"}]}}`
	for i := range widget.MaxIDs {
		res, err := a.ShowWidget(widgetArgs(t, kv))
		if err != nil || res != fmt.Sprintf("shown w%d", i+1) {
			t.Fatalf("widget %d gave %q, %v", i+1, res, err)
		}
	}
	if _, err := a.ShowWidget(widgetArgs(t, kv)); err == nil {
		t.Fatal("a 51st widget was shown")
	}
	if res, err := a.ShowWidget(widgetArgs(t, `{"id":"w7",`+kv[1:])); err != nil || res != "updated w7" {
		t.Fatalf("updating at the limit gave %q, %v", res, err)
	}
}

func TestAClickDuringAPermissionPromptWaitsAndNeverDeniesIt(t *testing.T) {
	a, d := busyAgent(t)
	if _, err := a.ShowWidget(widgetArgs(t, `{"id":"env","kind":"form","form":{"fields":[{"name":"env","label":"Env","kind":"choice","options":["dev","prod"]}]}}`)); err != nil {
		t.Fatal(err)
	}
	a.event(func() bool { a.ask(&Pending{ReqID: "towel", Tool: "Bash"}); return true })
	a.AnswerWidget("env", widget.Answer{Values: map[string]string{"env": "prod"}})
	s := a.Snapshot()
	if s.State != NeedsYou || len(s.Pending) != 1 {
		t.Fatalf("the click changed the prompt: %s with %d pending", s.State, len(s.Pending))
	}
	if len(s.Queued) != 1 || s.Queued[0].From != "widget" {
		t.Fatalf("the click was not held: %+v", s.Queued)
	}
	a.Answer("towel", true, false, nil)
	endTurn(a)
	waitFor(t, a, "the held click was delivered", func(Snapshot) bool { return len(d.delivered()) == 2 })
	if got := d.delivered()[1]; !strings.Contains(got, `{"values":{"env":"prod"}}`) {
		t.Fatalf("after the turn the agent got %q", got)
	}
	if u := userTexts(a.Snapshot()); len(u) != 1 {
		t.Fatalf("You rows: %q", u)
	}
}

func TestClearingAPendingClickMakesTheWidgetAnswerableAgain(t *testing.T) {
	a, _ := busyAgent(t)
	if _, err := a.ShowWidget(widgetArgs(t, `{"id":"go","kind":"buttons","buttons":{"buttons":[{"label":"Go"}]}}`)); err != nil {
		t.Fatal(err)
	}
	a.AnswerWidget("go", widget.Answer{Clicked: "Go"})
	s := a.Snapshot()
	if len(s.Queued) != 1 {
		t.Fatalf("%d pending", len(s.Queued))
	}
	if _, ok := a.BeginEdit(s.Queued[0].ID); ok {
		t.Fatal("a pending click opened for editing")
	}
	a.ClearQueued(s.Queued[0].ID)
	if e := a.Snapshot().Entries[1]; e.Answer != nil {
		t.Fatal("the widget stayed answered after its click was cleared")
	}
}

func rpc(t *testing.T, m *mcpServer, path, method string, params any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	rec := httptest.NewRecorder()
	m.serve(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		return map[string]any{"status": rec.Code}
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func toolNames(r map[string]any) []string {
	var out []string
	res, _ := r["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	for _, x := range tools {
		out = append(out, x.(map[string]any)["name"].(string))
	}
	return out
}

func TestEachSessionsToolServerOffersOnlyShowWidgetForThatSession(t *testing.T) {
	h := newHub(nil, "")
	arthur := wallSession(t, h, Meta{Title: "Arthur", Cwd: t.TempDir()})
	ford := wallSession(t, h, Meta{Title: "Ford", Cwd: t.TempDir()})
	m := &mcpServer{hub: h, base: "http://x/mcp/", disp: "disp", tokens: map[string]*Agent{}, byAgent: map[*Agent]string{}}
	h.mcp = m
	path := strings.TrimPrefix(m.sessionURL(arthur), "http://x")

	if got := toolNames(rpc(t, m, path, "tools/list", nil)); len(got) != 1 || got[0] != widgetTool {
		t.Fatalf("a session's server lists %q", got)
	}
	if got := toolNames(rpc(t, m, "/mcp/disp", "tools/list", nil)); len(got) < 2 || got[len(got)-1] != widgetTool {
		t.Fatalf("the dispatcher's server lists %q", got)
	}
	r := rpc(t, m, path, "tools/call", map[string]any{"name": "stop_session", "arguments": map[string]any{"session": "Ford"}})
	if res, _ := r["result"].(map[string]any); res["isError"] != true {
		t.Fatalf("a session called stop_session: %v", r)
	}
	rpc(t, m, path, "tools/call", map[string]any{"name": widgetTool, "arguments": map[string]any{"kind": "keyvalue", "keyvalue": map[string]any{"items": []any{map[string]any{"key": "towel", "value": "yes"}}}}})
	if len(arthur.Snapshot().Entries) != 1 || len(ford.Snapshot().Entries) != 0 {
		t.Fatal("the widget did not land in the calling session only")
	}
	if r := rpc(t, m, "/mcp/guess", "tools/list", nil); r["status"] != http.StatusNotFound {
		t.Fatalf("an unknown path answered: %v", r)
	}
	args := strings.Join(h.sessionMCPArgs(arthur), " ")
	if !strings.Contains(args, m.sessionURL(arthur)) || !strings.Contains(args, "--allowedTools "+widgetToolFull) {
		t.Fatalf("session launch args: %s", args)
	}
	h.Remove(arthur, false)
	if r := rpc(t, m, path, "tools/list", nil); r["status"] != http.StatusNotFound {
		t.Fatal("a removed session's server still answers")
	}
}

func TestWidgetsAndAnswersComeBackFromTheClaudeTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := newHub(nil, "")
	a := wallSession(t, h, Meta{Title: "Marvin", Cwd: "/tmp/heart-of-gold"})
	lines := []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": "Pick a planet"}},
		{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": widgetToolFull,
			"input": map[string]any{"kind": "buttons", "buttons": map[string]any{"buttons": []any{map[string]any{"label": "Magrathea"}, map[string]any{"label": "Earth"}}}}}}}},
		{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "shown w1"}}}},
		{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "t2", "name": widgetToolFull,
			"input": map[string]any{"id": "eta", "kind": "progress", "progress": map[string]any{"bars": []any{map[string]any{"label": "Build", "value": 0.2}}}}}}}},
		{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t2", "content": "shown eta"}}}},
		{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "t3", "name": widgetToolFull,
			"input": map[string]any{"id": "eta", "kind": "progress", "progress": map[string]any{"bars": []any{map[string]any{"label": "Build", "value": 0.9}}}}}}}},
		{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t3", "content": "updated eta"}}}},
		{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "t4", "name": widgetToolFull,
			"input": map[string]any{"kind": "table"}}}}},
		{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t4", "content": "kind table needs a table object", "is_error": true}}}},
		{"type": "user", "message": map[string]any{"role": "user", "content": `<switchboard-widget id="w1" kind="buttons">{"clicked":"Earth"}</switchboard-widget>` + "\n\nAnd hurry"}},
	}
	var b bytes.Buffer
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	dir := filepath.Join(home, ".claude", "projects", slugRE.ReplaceAllString(a.Meta.Cwd, "-"))
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, a.Meta.ID+".jsonl"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeDriver{}.history(a)

	var widgets []Entry
	s := a.Snapshot()
	for _, e := range s.Entries {
		switch e.Kind {
		case EntryWidget:
			widgets = append(widgets, e)
		case EntryTool:
			t.Fatalf("a widget call showed as a tool row: %+v", e)
		}
	}
	if len(widgets) != 2 {
		t.Fatalf("%d widgets came back", len(widgets))
	}
	if widgets[0].Answer == nil || widgets[0].Answer.Clicked != "Earth" {
		t.Fatal("the click on w1 did not come back")
	}
	if v := widgets[1].Widget.Progress.Bars[0].Value; v != 0.9 {
		t.Fatalf("the progress widget came back at %v", v)
	}
	if u := userTexts(s); len(u) != 2 || u[1] != "And hurry" {
		t.Fatalf("You rows: %q", u)
	}
	if _, err := a.ShowWidget(widgetArgs(t, `{"id":"w1","kind":"buttons","buttons":{"buttons":[{"label":"Again"}]}}`)); err == nil {
		t.Fatal("an answered widget from before the restart was replaced")
	}
}
