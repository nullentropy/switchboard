package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type codexDriver struct {
	files map[string]string
}

type codexAsk struct {
	id      json.RawMessage
	method  string
	qids    map[string]string
	granted any
}

func codexPolicy(mode string) (string, string, map[string]any) {
	switch mode {
	case "plan":
		return "on-request", "read-only", map[string]any{"type": "readOnly", "networkAccess": false}
	case "bypassPermissions":
		return "never", "danger-full-access", map[string]any{"type": "dangerFullAccess"}
	case "acceptEdits", "auto":
		return "on-request", "workspace-write", codexWorkspace()
	}
	return "untrusted", "workspace-write", codexWorkspace()
}

func codexWorkspace() map[string]any {
	return map[string]any{"type": "workspaceWrite", "writableRoots": []string{}, "networkAccess": false,
		"excludeTmpdirEnvVar": false, "excludeSlashTmp": false}
}

func (d *codexDriver) launch(a *Agent) (*proc, error) {
	a.mu.Lock()
	m := a.Meta
	a.mu.Unlock()
	var p *proc
	p, err := startProc(a, a.hub.bin("codex"), []string{"app-server"}, func(line []byte) {
		if msg, ok := p.route(line); ok {
			d.handle(a, p, msg)
		}
	})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*proc, error) {
		p.stop()
		return nil, err
	}
	if _, err := p.call("initialize", map[string]any{"clientInfo": map[string]any{"name": "switchboard", "title": "Switchboard", "version": "0.1"}}, 30*time.Second); err != nil {
		return fail(err)
	}
	_ = p.notify("initialized", nil)
	if raw, err := p.call("account/read", map[string]any{"refreshToken": false}, 30*time.Second); err == nil {
		var acct struct {
			Account       any  `json:"account"`
			RequiresLogin bool `json:"requiresOpenaiAuth"`
		}
		_ = json.Unmarshal(raw, &acct)
		if acct.RequiresLogin && acct.Account == nil {
			return fail(errors.New("Codex is not logged in. Run `codex login` in a terminal, then send again."))
		}
	}
	go func() {
		if raw, err := p.call("account/rateLimits/read", nil, 30*time.Second); err == nil {
			codexLimits(a.hub, raw)
		}
	}()
	approval, sandbox, _ := codexPolicy(m.Mode)
	params := map[string]any{"cwd": m.Cwd, "approvalPolicy": approval, "sandbox": sandbox}
	if m.Model != "" {
		params["model"] = m.Model
	}
	if m.Effort != "" {
		params["config"] = map[string]any{"model_reasoning_effort": m.Effort}
	}
	method := "thread/start"
	if m.Thread != "" {
		method = "thread/resume"
		params["threadId"] = m.Thread
	}
	raw, err := p.call(method, params, 60*time.Second)
	if err != nil {
		return fail(fmt.Errorf("start a Codex thread: %w", err))
	}
	var res struct {
		Thread struct {
			ID    string `json:"id"`
			Turns []struct {
				Items []map[string]any `json:"items"`
			} `json:"turns"`
		} `json:"thread"`
		Effort string `json:"reasoningEffort"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || res.Thread.ID == "" {
		return fail(fmt.Errorf("Codex returned no thread"))
	}
	a.mu.Lock()
	a.Meta.Thread = res.Thread.ID
	a.effortSeen = res.Effort
	if len(a.Entries) <= 1 && len(res.Thread.Turns) > 0 {
		cur := a.Entries
		a.Entries = nil
		a.replaying = true
		for _, t := range res.Thread.Turns {
			for _, it := range t.Items {
				d.item(a, it, true)
			}
		}
		a.replaying = false
		a.Entries = append(a.Entries, cur...)
		a.toolIdx = map[string]int{}
	}
	a.mu.Unlock()
	go a.hub.save()
	return p, nil
}

func (d *codexDriver) send(a *Agent, text string, imgs []Image) error {
	a.mu.Lock()
	thread, mode, effort, p := a.Meta.Thread, a.Meta.Mode, a.Meta.Effort, a.p
	a.mu.Unlock()
	approval, _, sandbox := codexPolicy(mode)
	input, err := codexInput(text, imgs, imageDir())
	if err != nil {
		return err
	}
	params := map[string]any{"threadId": thread, "approvalPolicy": approval, "sandboxPolicy": sandbox, "input": input}
	if effort != "" {
		params["effort"] = effort
	}
	raw, err := p.call("turn/start", params, 60*time.Second)
	if err != nil {
		return err
	}
	var res struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(raw, &res)
	a.mu.Lock()
	if a.turn == "" && a.State != Ready {
		a.turn = res.Turn.ID
	}
	if effort != "" {
		a.effortSeen = effort
	}
	a.mu.Unlock()
	return nil
}

func (d *codexDriver) interrupt(a *Agent) {
	a.mu.Lock()
	thread, turn, p := a.Meta.Thread, a.turn, a.p
	a.mu.Unlock()
	if p != nil && turn != "" {
		go p.call("turn/interrupt", map[string]any{"threadId": thread, "turnId": turn}, 30*time.Second)
	}
}

func (d *codexDriver) setMode(a *Agent, mode string) {}

// turn/start carries Meta.Effort, and codex keeps it for later turns.
func (d *codexDriver) setEffort(a *Agent, effort string) {}

func (d *codexDriver) history(a *Agent) {}

func codexCommand(c string) string {
	for _, sh := range []string{"/bin/zsh -lc ", "/bin/bash -lc ", "bash -lc ", "zsh -lc ", "sh -c "} {
		if strings.HasPrefix(c, sh) {
			c = strings.Trim(strings.TrimPrefix(c, sh), `'"`)
			break
		}
	}
	return c
}

func (d *codexDriver) toolOf(it map[string]any) (name, sum, station string) {
	str := func(k string) string { s, _ := it[k].(string); return s }
	switch str("type") {
	case "commandExecution":
		cmd := codexCommand(str("command"))
		if acts, _ := it["commandActions"].([]any); len(acts) == 1 {
			act, _ := acts[0].(map[string]any)
			path, _ := act["path"].(string)
			switch act["type"] {
			case "read":
				return "Read", shortPath(path), "Read"
			case "search":
				q, _ := act["query"].(string)
				return "Grep", oneLine(q), "Read"
			case "listFiles":
				return "Glob", shortPath(path), "Read"
			}
		}
		return "Bash", oneLine(cmd), commandStation(cmd)
	case "fileChange":
		var paths []string
		changes, _ := it["changes"].([]any)
		for _, c := range changes {
			ch, _ := c.(map[string]any)
			p, _ := ch["path"].(string)
			paths = append(paths, shortPath(p))
		}
		s := strings.Join(paths, ", ")
		if d.files == nil {
			d.files = map[string]string{}
		}
		d.files[str("id")] = s
		return "Edit", oneLine(s), "Code"
	case "mcpToolCall":
		return str("server") + "." + str("tool"), "", "Run"
	case "webSearch":
		return "WebSearch", oneLine(str("query")), "Read"
	}
	return "", "", ""
}

func (d *codexDriver) item(a *Agent, it map[string]any, done bool) bool {
	typ, _ := it["type"].(string)
	id, _ := it["id"].(string)
	switch typ {
	case "agentMessage":
		if done {
			t, _ := it["text"].(string)
			a.say(t)
		}
		return done
	case "userMessage":
		if a.replaying && done {
			content, _ := it["content"].([]any)
			for _, c := range content {
				cm, _ := c.(map[string]any)
				if t, _ := cm["text"].(string); t != "" {
					a.addEntry(Entry{Kind: EntryUser, Text: t})
				}
			}
		}
		return false
	case "reasoning":
		if !done {
			a.think()
		}
		return !done
	}
	name, sum, station := d.toolOf(it)
	if name == "" {
		return false
	}
	if _, ok := a.toolIdx[id]; !ok {
		a.toolStart(id, name, sum, station)
	}
	if !done {
		return true
	}
	status, _ := it["status"].(string)
	detail, _ := it["aggregatedOutput"].(string)
	if typ == "fileChange" {
		var b strings.Builder
		changes, _ := it["changes"].([]any)
		for _, c := range changes {
			ch, _ := c.(map[string]any)
			diff, _ := ch["diff"].(string)
			b.WriteString(diff)
		}
		detail = b.String()
	}
	if e, ok := it["error"].(map[string]any); ok {
		detail, _ = e["message"].(string)
	}
	if status == "declined" {
		detail = "Declined"
	}
	a.toolEnd(id, detail, status == "failed" || status == "declined")
	return true
}

func (d *codexDriver) handle(a *Agent, p *proc, msg rpcMsg) {
	var params map[string]any
	_ = json.Unmarshal(msg.Params, &params)
	if len(msg.ID) > 0 {
		d.request(a, p, msg, params)
		return
	}
	a.mu.Lock()
	thread := a.Meta.Thread
	a.mu.Unlock()
	if t, _ := params["threadId"].(string); t != "" && thread != "" && t != thread {
		return
	}
	switch msg.Method {
	case "account/rateLimits/updated":
		codexLimits(a.hub, msg.Params)
	case "thread/tokenUsage/updated":
		a.event(func() bool { return codexContext(a, params) })
	case "item/started", "item/completed":
		it, _ := params["item"].(map[string]any)
		a.event(func() bool { return d.item(a, it, msg.Method == "item/completed") })
	case "turn/started":
		turn, _ := params["turn"].(map[string]any)
		id, _ := turn["id"].(string)
		a.event(func() bool { a.turn = id; a.live(); return true })
	case "turn/completed":
		turn, _ := params["turn"].(map[string]any)
		errText := ""
		if turn["status"] == "failed" {
			if e, ok := turn["error"].(map[string]any); ok {
				errText, _ = e["message"].(string)
			}
			if errText == "" {
				errText = "The turn failed."
			}
		}
		a.event(func() bool { a.turnEnd(errText); return true })
	}
}

func (d *codexDriver) request(a *Agent, p *proc, msg rpcMsg, params map[string]any) {
	str := func(k string) string { s, _ := params[k].(string); return s }
	ask := &codexAsk{id: msg.ID, method: msg.Method}
	pd := &Pending{ReqID: string(msg.ID), raw: ask}
	switch msg.Method {
	case "item/commandExecution/requestApproval":
		cmd := codexCommand(str("command"))
		pd.Tool, pd.Summary, pd.Input, pd.CanAlways = "Bash", oneLine(cmd), map[string]any{"command": cmd}, true
	case "item/fileChange/requestApproval":
		a.mu.Lock()
		files := d.files[str("itemId")]
		a.mu.Unlock()
		if files == "" {
			files = str("grantRoot")
		}
		pd.Tool, pd.Summary, pd.CanAlways = "Edit", files, true
	case "execCommandApproval":
		var parts []string
		cmd, _ := params["command"].([]any)
		for _, c := range cmd {
			s, _ := c.(string)
			parts = append(parts, s)
		}
		pd.Tool, pd.Summary, pd.CanAlways = "Bash", oneLine(strings.Join(parts, " ")), true
	case "applyPatchApproval":
		pd.Tool, pd.Summary, pd.CanAlways = "Edit", oneLine(str("reason")), true
	case "item/permissions/requestApproval":
		pd.Tool, pd.Summary = "Permissions", oneLine(str("reason"))
		ask.granted = params["permissions"]
	case "item/tool/requestUserInput":
		qs, _ := params["questions"].([]any)
		ask.qids = map[string]string{}
		for _, q := range qs {
			qm, _ := q.(map[string]any)
			text, _ := qm["question"].(string)
			header, _ := qm["header"].(string)
			id, _ := qm["id"].(string)
			ask.qids[text] = id
			question := Question{Question: text, Header: header}
			opts, _ := qm["options"].([]any)
			for _, o := range opts {
				om, _ := o.(map[string]any)
				l, _ := om["label"].(string)
				desc, _ := om["description"].(string)
				question.Options = append(question.Options, Option{Label: l, Description: desc})
			}
			pd.Questions = append(pd.Questions, question)
		}
		pd.Tool = "AskUserQuestion"
	case "mcpServer/elicitation/request":
		_ = p.reply(msg.ID, map[string]any{"action": "decline", "content": nil, "_meta": nil})
		return
	default:
		_ = p.replyError(msg.ID, "switchboard does not handle "+msg.Method)
		return
	}
	a.event(func() bool { a.ask(pd); return true })
}

func (d *codexDriver) answer(a *Agent, pd *Pending, allow, always bool, answers map[string]string, reason string) {
	ask, _ := pd.raw.(*codexAsk)
	p := a.proc()
	if ask == nil || p == nil {
		return
	}
	var result any
	switch ask.method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		dec := "decline"
		if allow {
			dec = "accept"
			if always {
				dec = "acceptForSession"
			}
		}
		result = map[string]any{"decision": dec}
	case "execCommandApproval", "applyPatchApproval":
		if reason == "" {
			reason = "The user declined this."
		}
		var dec any = map[string]any{"denied": map[string]any{"rejection": reason}}
		if allow {
			dec = "approved"
			if always {
				dec = "approved_for_session"
			}
		}
		result = map[string]any{"decision": dec}
	case "item/permissions/requestApproval":
		perms := map[string]any{}
		if allow {
			if g, ok := ask.granted.(map[string]any); ok {
				for k, v := range g {
					if v != nil {
						perms[k] = v
					}
				}
			}
		}
		result = map[string]any{"permissions": perms, "scope": "turn"}
	case "item/tool/requestUserInput":
		out := map[string]any{}
		for q, ans := range answers {
			if id := ask.qids[q]; id != "" {
				out[id] = map[string]any{"answers": strings.Split(ans, ", ")}
			}
		}
		result = map[string]any{"answers": out}
	}
	_ = p.reply(ask.id, result)
}
