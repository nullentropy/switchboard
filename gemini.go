package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type geminiDriver struct {
	msg     strings.Builder
	loading bool
}

type geminiAsk struct {
	id      json.RawMessage
	options []map[string]any
}

func geminiMode(mode string) string {
	switch mode {
	case "acceptEdits", "auto":
		return "autoEdit"
	case "plan":
		return "plan"
	case "bypassPermissions":
		return "yolo"
	}
	return "default"
}

func (d *geminiDriver) launch(a *Agent) (*proc, error) {
	a.mu.Lock()
	m := a.Meta
	a.mu.Unlock()
	args := []string{"--acp"}
	if m.Model != "" {
		args = append(args, "-m", m.Model)
	}
	var p *proc
	p, err := startProc(a, a.hub.bin("gemini"), args, func(line []byte) {
		if msg, ok := p.route(line); ok {
			d.handle(a, p, msg)
		}
	})
	if err != nil {
		return nil, err
	}
	p.rpcTag = true
	fail := func(err error) (*proc, error) {
		p.stop()
		return nil, err
	}
	if _, err := p.call("initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "switchboard", "version": "0.1"}}, 60*time.Second); err != nil {
		return fail(err)
	}
	session := m.Thread
	if session != "" {
		a.mu.Lock()
		cur := a.Entries
		a.Entries = nil
		a.replaying = true
		d.loading = true
		a.mu.Unlock()
		_, err := p.call("session/load", map[string]any{"sessionId": session, "cwd": m.Cwd, "mcpServers": []any{}}, 120*time.Second)
		a.mu.Lock()
		d.flush(a)
		d.loading = false
		a.replaying = false
		if len(cur) <= 1 {
			a.Entries = append(a.Entries, cur...)
		} else {
			a.Entries = cur
		}
		a.toolIdx = map[string]int{}
		a.mu.Unlock()
		if err != nil {
			session = ""
		}
	}
	if session == "" {
		raw, err := p.call("session/new", map[string]any{"cwd": m.Cwd, "mcpServers": []any{}}, 120*time.Second)
		if err != nil {
			if strings.Contains(err.Error(), "missing or not configured") || strings.Contains(err.Error(), "auth") {
				return fail(errors.New("Gemini is not logged in. Run `gemini` in a terminal once to log in, or set GEMINI_API_KEY, then send again."))
			}
			return fail(fmt.Errorf("start a Gemini session: %w", err))
		}
		var res struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(raw, &res)
		if res.SessionID == "" {
			return fail(errors.New("Gemini returned no session"))
		}
		session = res.SessionID
	}
	a.mu.Lock()
	a.Meta.Thread = session
	a.mu.Unlock()
	go a.hub.save()
	if mode := geminiMode(m.Mode); mode != "default" {
		if _, err := p.call("session/set_mode", map[string]any{"sessionId": session, "modeId": mode}, 30*time.Second); err != nil {
			a.event(func() bool {
				a.addEntry(Entry{Kind: EntryError, Text: "Gemini would not switch to " + mode + " here, so it asks first. Gemini only allows that mode in a folder you have trusted in the gemini CLI."})
				return true
			})
		}
	}
	return p, nil
}

func (d *geminiDriver) send(a *Agent, text string, imgs []Image) error {
	a.mu.Lock()
	session, p := a.Meta.Thread, a.p
	a.mu.Unlock()
	prompt := geminiPrompt(text, imgs)
	go func() {
		raw, err := p.call("session/prompt", map[string]any{"sessionId": session,
			"prompt": prompt}, 24*time.Hour)
		var res struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(raw, &res)
		a.event(func() bool {
			d.flush(a)
			errText := ""
			switch {
			case err != nil && errors.Is(err, errExited):
				return false
			case err != nil:
				errText = err.Error()
			case res.StopReason == "max_tokens":
				errText = "Gemini ran out of context."
			case res.StopReason == "max_turn_requests":
				errText = "Gemini stopped after too many steps in one turn."
			case res.StopReason == "refusal":
				errText = "Gemini refused."
			}
			a.turnEnd(errText)
			return true
		})
	}()
	return nil
}

func (d *geminiDriver) interrupt(a *Agent) {
	a.mu.Lock()
	session, p := a.Meta.Thread, a.p
	var asks []*geminiAsk
	for _, pd := range a.Pending {
		if g, ok := pd.raw.(*geminiAsk); ok {
			asks = append(asks, g)
		}
	}
	a.Pending = nil
	a.mu.Unlock()
	if p == nil {
		return
	}
	for _, g := range asks {
		_ = p.reply(g.id, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
	}
	_ = p.notify("session/cancel", map[string]any{"sessionId": session})
}

func (d *geminiDriver) setMode(a *Agent, mode string) {
	a.mu.Lock()
	session, p := a.Meta.Thread, a.p
	a.mu.Unlock()
	if p != nil && session != "" {
		go p.call("session/set_mode", map[string]any{"sessionId": session, "modeId": geminiMode(mode)}, 30*time.Second)
	}
}

func (d *geminiDriver) history(a *Agent) {}

func (d *geminiDriver) flush(a *Agent) {
	if d.msg.Len() > 0 {
		a.say(d.msg.String())
		d.msg.Reset()
	}
}

func geminiTool(call map[string]any) (name, sum, station string) {
	kind, _ := call["kind"].(string)
	title, _ := call["title"].(string)
	path := ""
	if locs, _ := call["locations"].([]any); len(locs) > 0 {
		l, _ := locs[0].(map[string]any)
		path, _ = l["path"].(string)
	}
	target := oneLine(title)
	if path != "" {
		target = shortPath(path)
	}
	switch kind {
	case "read":
		return "Read", target, "Read"
	case "search":
		return "Grep", oneLine(title), "Read"
	case "edit":
		return "Edit", target, "Code"
	case "delete":
		return "Delete", target, "Code"
	case "move":
		return "Move", target, "Code"
	case "execute":
		return "Bash", oneLine(title), commandStation(title)
	case "fetch":
		return "WebFetch", oneLine(title), "Read"
	case "think":
		return "Agent", oneLine(title), "Think"
	}
	return "Tool", oneLine(title), "Run"
}

func geminiContent(call map[string]any) string {
	var b strings.Builder
	items, _ := call["content"].([]any)
	for _, c := range items {
		cm, _ := c.(map[string]any)
		switch cm["type"] {
		case "content":
			inner, _ := cm["content"].(map[string]any)
			t, _ := inner["text"].(string)
			b.WriteString(t)
			b.WriteByte('\n')
		case "diff":
			path, _ := cm["path"].(string)
			newText, _ := cm["newText"].(string)
			fmt.Fprintf(&b, "%s\n%s\n", shortPath(path), newText)
		}
	}
	return b.String()
}

func (d *geminiDriver) handle(a *Agent, p *proc, msg rpcMsg) {
	var params map[string]any
	_ = json.Unmarshal(msg.Params, &params)
	switch msg.Method {
	case "session/update":
		up, _ := params["update"].(map[string]any)
		a.event(func() bool { return d.update(a, up) })
	case "session/request_permission":
		call, _ := params["toolCall"].(map[string]any)
		name, sum, _ := geminiTool(call)
		ask := &geminiAsk{id: msg.ID}
		opts, _ := params["options"].([]any)
		canAlways := false
		for _, o := range opts {
			om, _ := o.(map[string]any)
			ask.options = append(ask.options, om)
			if om["kind"] == "allow_always" {
				canAlways = true
			}
		}
		pd := &Pending{ReqID: string(msg.ID), Tool: name, Summary: sum, CanAlways: canAlways, raw: ask}
		if name == "Bash" {
			pd.Input = map[string]any{"command": sum}
		}
		a.event(func() bool { d.flush(a); a.ask(pd); return true })
	default:
		if len(msg.ID) > 0 {
			_ = p.replyError(msg.ID, "switchboard does not handle "+msg.Method)
		}
	}
}

func (d *geminiDriver) update(a *Agent, up map[string]any) bool {
	kind, _ := up["sessionUpdate"].(string)
	text := func() string {
		c, _ := up["content"].(map[string]any)
		t, _ := c["text"].(string)
		return t
	}
	switch kind {
	case "user_message_chunk":
		if d.loading {
			d.flush(a)
			if e, ok := geminiReplayUser(up["content"]); ok {
				a.addEntry(e)
			}
		}
		return false
	case "agent_message_chunk":
		if meta, _ := up["_meta"].(map[string]any); meta != nil && meta["reset"] == true {
			d.msg.Reset()
		}
		d.msg.WriteString(text())
		if !a.replaying && a.Activity != "Writing" {
			a.Activity = "Writing"
			a.live()
			return true
		}
		return false
	case "agent_thought_chunk":
		d.flush(a)
		if a.Activity != "Thinking" {
			a.think()
			return true
		}
		return false
	case "tool_call":
		d.flush(a)
		id, _ := up["toolCallId"].(string)
		name, sum, station := geminiTool(up)
		if _, ok := a.toolIdx[id]; !ok {
			a.toolStart(id, name, sum, station)
		}
		return d.finish(a, id, up) || true
	case "tool_call_update":
		id, _ := up["toolCallId"].(string)
		return d.finish(a, id, up)
	}
	return false
}

func (d *geminiDriver) finish(a *Agent, id string, up map[string]any) bool {
	status, _ := up["status"].(string)
	if status != "completed" && status != "failed" {
		return false
	}
	return a.toolEnd(id, geminiContent(up), status == "failed")
}

func (d *geminiDriver) answer(a *Agent, pd *Pending, allow, always bool, _ map[string]string, _ string) {
	ask, _ := pd.raw.(*geminiAsk)
	p := a.proc()
	if ask == nil || p == nil {
		return
	}
	want := []string{"reject_once", "reject_always"}
	if allow {
		want = []string{"allow_once", "allow_always"}
		if always {
			want = []string{"allow_always", "allow_once"}
		}
	}
	for _, k := range want {
		for _, o := range ask.options {
			if o["kind"] == k {
				_ = p.reply(ask.id, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": o["optionId"]}})
				return
			}
		}
	}
	_ = p.reply(ask.id, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
}
