package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	switch os.Getenv("SWITCHBOARD_FAKE") {
	case "claude":
		fakeClaude()
	case "codex":
		fakeCodex()
	case "gemini":
		fakeGemini()
	default:
		os.Exit(m.Run())
	}
	os.Exit(0)
}

type fakeIO struct {
	in  *bufio.Scanner
	out *bufio.Writer
}

func newFakeIO() *fakeIO {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	return &fakeIO{in: sc, out: bufio.NewWriter(os.Stdout)}
}

func (f *fakeIO) read() map[string]any {
	if !f.in.Scan() {
		os.Exit(0)
	}
	var m map[string]any
	_ = json.Unmarshal(f.in.Bytes(), &m)
	return m
}

func (f *fakeIO) write(v any) {
	b, _ := json.Marshal(v)
	f.out.Write(append(b, '\n'))
	f.out.Flush()
}

func (f *fakeIO) until(method string) map[string]any {
	for {
		if m := f.read(); m["method"] == method {
			return m
		}
	}
}

func fakeRecord(line string) {
	if path := os.Getenv("SWITCHBOARD_FAKE_ARGS"); path != "" {
		if fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(fh, line)
			fh.Close()
		}
	}
}

func fakeClaude() {
	fakeRecord(strings.Join(os.Args[1:], " "))
	effort := "medium"
	for i, a := range os.Args {
		if a == "--effort" && i+1 < len(os.Args) {
			effort = os.Args[i+1]
		}
	}
	f := newFakeIO()
	settings := func(m map[string]any) bool {
		r, _ := m["request"].(map[string]any)
		if m["type"] != "control_request" {
			return false
		}
		switch r["subtype"] {
		case "apply_flag_settings":
			settings, _ := r["settings"].(map[string]any)
			if e, ok := settings["effortLevel"].(string); ok {
				effort = e
			}
			f.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"]}})
			return true
		case "get_settings":
			f.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"],
				"response": map[string]any{"applied": map[string]any{"model": "deep-thought", "effort": effort}}}})
			return true
		}
		return false
	}
	f.write(map[string]any{"type": "system", "subtype": "init"})
	for {
		m := f.read()
		if settings(m) {
			continue
		}
		if r, _ := m["request"].(map[string]any); m["type"] == "control_request" && r["subtype"] == "initialize" {
			f.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"],
				"response": map[string]any{"commands": []any{
					map[string]any{"name": "review", "description": "Review the branch (project)", "argumentHint": "[pr]"},
					map[string]any{"name": "my-tickets", "description": "List my tickets (user)", "argumentHint": ""},
					map[string]any{"name": "ne:sietch", "description": "Keep a track's record (plugin:ne)", "argumentHint": ""},
					map[string]any{"name": "advisor", "description": "Consult a stronger model", "argumentHint": ""},
					map[string]any{"name": "__remote-workflow", "description": "Run a script (server-launched sessions only)", "argumentHint": ""},
					map[string]any{"name": "agents", "description": "(removed) Manage subagents", "argumentHint": ""},
				}, "models": []any{
					map[string]any{"value": "default", "displayName": "Default (recommended)"},
					map[string]any{"value": "deep-thought", "displayName": "Deep Thought 42"},
					map[string]any{"value": "heart-of-gold", "displayName": "Heart of Gold"},
				}}}})
			continue
		}
		if m["type"] != "user" {
			continue
		}
		f.write(map[string]any{"type": "control_request", "request_id": "r1", "request": map[string]any{
			"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]any{"command": "go test ./..."}}})
		resp := f.read()
		for settings(resp) {
			resp = f.read()
		}
		r, _ := resp["response"].(map[string]any)
		inner, _ := r["response"].(map[string]any)
		f.write(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{"command": "go test ./..."}}}}})
		f.write(map[string]any{"type": "user", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "ok"}}}})
		f.write(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": strings.TrimSuffix(fmt.Sprint("decision ", inner["behavior"], ": ", inner["message"]), ": <nil>")}}}})
		f.write(map[string]any{"type": "result", "subtype": "success", "total_cost_usd": 0.25})
	}
}

func fakeCodex() {
	f := newFakeIO()
	reply := func(m map[string]any, result any) { f.write(map[string]any{"id": m["id"], "result": result}) }
	reply(f.until("initialize"), map[string]any{"userAgent": "fake"})
	reply(f.until("account/read"), map[string]any{"account": map[string]any{"type": "chatgpt"}, "requiresOpenaiAuth": true})
	start := f.until("thread/start")
	params, _ := start["params"].(map[string]any)
	config, _ := params["config"].(map[string]any)
	effort, _ := config["model_reasoning_effort"].(string)
	fakeRecord("thread/start model_reasoning_effort=" + effort)
	if effort == "" {
		effort = "medium"
	}
	reply(start, map[string]any{"thread": map[string]any{"id": "th1"}, "reasoningEffort": effort})
	for {
		m := f.until("turn/start")
		params, _ := m["params"].(map[string]any)
		fakeRecord(fmt.Sprint("turn/start effort=", params["effort"]))
		reply(m, map[string]any{"turn": map[string]any{"id": "tu1", "status": "inProgress"}})
		note := func(method string, params map[string]any) {
			params["threadId"], params["turnId"] = "th1", "tu1"
			f.write(map[string]any{"method": method, "params": params})
		}
		note("turn/started", map[string]any{"turn": map[string]any{"id": "tu1"}})
		cmd := map[string]any{"type": "commandExecution", "id": "c1", "command": "/bin/zsh -lc 'go test ./...'", "status": "inProgress",
			"commandActions": []any{map[string]any{"type": "unknown", "command": "go test ./..."}}}
		note("item/started", map[string]any{"item": cmd})
		f.write(map[string]any{"id": 0, "method": "item/commandExecution/requestApproval", "params": map[string]any{
			"threadId": "th1", "turnId": "tu1", "itemId": "c1", "command": "/bin/zsh -lc 'go test ./...'"}})
		resp := f.read()
		res, _ := resp["result"].(map[string]any)
		cmd["status"], cmd["aggregatedOutput"], cmd["exitCode"] = "completed", "ok", 0
		note("item/completed", map[string]any{"item": cmd})
		note("item/completed", map[string]any{"item": map[string]any{"type": "agentMessage", "id": "m1", "text": fmt.Sprint("decision ", res["decision"])}})
		note("turn/completed", map[string]any{"turn": map[string]any{"id": "tu1", "status": "completed"}})
	}
}

func fakeGemini() {
	f := newFakeIO()
	reply := func(m map[string]any, result any) {
		f.write(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": result})
	}
	reply(f.until("initialize"), map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}})
	reply(f.until("session/new"), map[string]any{"sessionId": "s1"})
	for {
		m := f.until("session/prompt")
		update := func(u map[string]any) {
			f.write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": u}})
		}
		update(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "**Plan**"}})
		call := map[string]any{"toolCallId": "x1", "status": "pending", "title": "go test ./...", "kind": "execute"}
		update(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "x1", "status": "pending", "title": "go test ./...", "kind": "execute"})
		f.write(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "session/request_permission", "params": map[string]any{
			"sessionId": "s1", "toolCall": call, "options": []any{
				map[string]any{"optionId": "proceed_always", "name": "Allow for this session", "kind": "allow_always"},
				map[string]any{"optionId": "proceed_once", "name": "Allow", "kind": "allow_once"},
				map[string]any{"optionId": "cancel", "name": "Reject", "kind": "reject_once"}}}})
		resp := f.read()
		res, _ := resp["result"].(map[string]any)
		outcome, _ := res["outcome"].(map[string]any)
		update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "x1", "status": "completed",
			"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "ok"}}}})
		update(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "decision "}})
		update(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": fmt.Sprint(outcome["optionId"])}})
		reply(m, map[string]any{"stopReason": "end_turn"})
	}
}

func waitFor(t *testing.T, a *Agent, what string, ok func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := a.Snapshot(); ok(s) {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := a.Snapshot()
	t.Fatalf("%s: state %s, err %q, entries %+v", what, s.State, s.Err, s.Entries)
	return s
}

func TestEveryProviderAsksBeforeACommandAndCarriesTheAnswerBack(t *testing.T) {
	for _, tc := range []struct{ provider, decision string }{
		{"claude", "decision allow"},
		{"codex", "decision accept"},
		{"gemini", "decision proceed_once"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			t.Setenv("SWITCHBOARD_FAKE", tc.provider)
			self, _ := os.Executable()
			h := newHub(map[string]string{"claude": self, "codex": self, "gemini": self}, "")
			a, _ := h.New(Meta{Provider: tc.provider, Cwd: t.TempDir(), Mode: "default"}, false)
			defer a.Stop()
			go a.Send("run the tests, and don't forget your towel")

			s := waitFor(t, a, "waiting on the user", func(s Snapshot) bool { return s.State == NeedsYou })
			p := s.Pending[0]
			if p.Tool != "Bash" || p.Summary != "go test ./..." {
				t.Fatalf("asked about %q %q, want Bash go test ./...", p.Tool, p.Summary)
			}
			a.Answer(p.ReqID, true, false, nil)

			s = waitFor(t, a, "turn end", func(s Snapshot) bool { return s.State == Ready })
			var tool, reply *Entry
			for i := range s.Entries {
				switch s.Entries[i].Kind {
				case EntryTool:
					tool = &s.Entries[i]
				case EntryAssistant:
					reply = &s.Entries[i]
				}
			}
			if tool == nil || tool.Text != "go test ./..." || strings.TrimSpace(tool.Detail) != "ok" || tool.Failed {
				t.Fatalf("tool entry %+v, want go test ./... with output ok", tool)
			}
			if reply == nil || reply.Text != tc.decision {
				t.Fatalf("reply %+v, want %q", reply, tc.decision)
			}
		})
	}
}

func TestAMessageSentOverAPendingApprovalDeniesItWithThatMessageAndCarriesOn(t *testing.T) {
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	self, _ := os.Executable()
	h := newHub(map[string]string{"claude": self}, "")
	a, _ := h.New(Meta{Cwd: t.TempDir(), Mode: "default"}, false)
	defer a.Stop()
	go a.Send("run the tests on the Heart of Gold")
	waitFor(t, a, "waiting on the user", func(s Snapshot) bool { return s.State == NeedsYou })

	go a.Send("Don't panic, ask Marvin instead")

	denied := func(s Snapshot) bool {
		for _, e := range s.Entries {
			if e.Kind == EntryAssistant && strings.HasPrefix(e.Text, "decision deny") && strings.Contains(e.Text, "Don't panic, ask Marvin instead") {
				return true
			}
		}
		return false
	}
	waitFor(t, a, "the approval denied with the new message", denied)
	waitFor(t, a, "the new message reaching the agent", func(s Snapshot) bool { return s.State == NeedsYou && len(s.Pending) == 1 })
}

func TestTheClaudeModelPickerListsTheModelsTheCLIOffers(t *testing.T) {
	self, _ := os.Executable()
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	h := newHub(map[string]string{"claude": self}, "")
	h.loadClaudeModels()
	got := fmt.Sprint(h.models("claude"))
	if want := "[deep-thought heart-of-gold]"; got != want {
		t.Fatalf("models %s, want %s", got, want)
	}
}

func TestAClaudeThatExitsWithoutAnsweringFallsBackToTheBuiltInModelsAndNoCommands(t *testing.T) {
	marvin := t.TempDir() + "/marvin"
	os.WriteFile(marvin, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	h := newHub(map[string]string{"claude": marvin}, "")
	h.loadClaudeModels()
	if got, want := fmt.Sprint(h.models("claude")), fmt.Sprint(staticModels["claude"]); got != want {
		t.Fatalf("models %s, want %s", got, want)
	}
	if got := h.commands("claude", t.TempDir()); got != nil {
		t.Fatalf("commands %+v, want none", got)
	}
}

func TestTypingASlashListsTheSessionsSkillsAndPluginCommands(t *testing.T) {
	self, _ := os.Executable()
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	h := newHub(map[string]string{"claude": self}, "")
	cmds := h.commands("claude", t.TempDir())
	if len(cmds) != 6 || cmds[2].Name != "ne:sietch" || cmds[0].Hint != "[pr]" {
		t.Fatalf("commands %+v", cmds)
	}
	if got := h.commands("codex", t.TempDir()); got != nil {
		t.Fatalf("codex has no commands to list, got %+v", got)
	}

	if _, ok := slashToken("fix /tmp please", -1); ok {
		t.Fatal("a slash inside a message is not a command")
	}
	if _, ok := slashToken("/my-tickets now", -1); ok {
		t.Fatal("a command with arguments typed is finished")
	}
	q, ok := slashToken("/si", -1)
	if !ok || q != "si" {
		t.Fatalf("query %q %v", q, ok)
	}
	if got := rankCommands(cmds, "si"); len(got) != 1 || got[0].Name != "ne:sietch" {
		t.Fatalf("ranked %+v", got)
	}
	if got := rankCommands(cmds, "my"); len(got) != 1 || got[0].Name != "my-tickets" {
		t.Fatalf("ranked %+v", got)
	}
	var names []string
	for _, c := range rankCommands(cmds, "") {
		names = append(names, c.Name)
	}
	if strings.Join(names, " ") != "my-tickets review ne:sietch advisor" {
		t.Fatalf("a bare slash lists %v", names)
	}
}
