package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type claudeDriver struct{}

func (claudeDriver) launch(a *Agent) (*proc, error) {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--permission-prompt-tool", "stdio"}
	a.mu.Lock()
	m := a.Meta
	a.mu.Unlock()
	if m.Mode != "" {
		args = append(args, "--permission-mode", m.Mode)
	}
	if m.Model != "" {
		args = append(args, "--model", m.Model)
	}
	if a.extraArgs != nil {
		args = append(args, a.extraArgs()...)
	}
	if m.Launched {
		args = append(args, "--resume", m.ID)
	} else {
		args = append(args, "--session-id", m.ID)
	}
	var p *proc
	p, err := startProc(a, a.hub.bin("claude"), args, func(line []byte) {
		var msg map[string]any
		if json.Unmarshal(line, &msg) != nil {
			return
		}
		a.event(func() bool { return claudeApply(a, p, msg) })
	})
	return p, err
}

func (claudeDriver) send(a *Agent, text string) error {
	return a.proc().writeJSON(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
}

func claudeControl(a *Agent, req map[string]any) {
	if p := a.proc(); p != nil {
		_ = p.writeJSON(map[string]any{"type": "control_request", "request_id": newID(), "request": req})
	}
}

func (claudeDriver) interrupt(a *Agent) { claudeControl(a, map[string]any{"subtype": "interrupt"}) }

func (claudeDriver) setMode(a *Agent, mode string) {
	claudeControl(a, map[string]any{"subtype": "set_permission_mode", "mode": mode})
}

func (claudeDriver) answer(a *Agent, p *Pending, allow, always bool, answers map[string]string) {
	var resp map[string]any
	if allow {
		input := p.Input
		if answers != nil {
			input = map[string]any{}
			for k, v := range p.Input {
				input[k] = v
			}
			input["answers"] = answers
		}
		resp = map[string]any{"behavior": "allow", "updatedInput": input}
		if always && len(p.Suggestions) > 0 {
			var perms []any
			for _, s := range p.Suggestions {
				if m, ok := s.(map[string]any); ok {
					c := map[string]any{}
					for k, v := range m {
						c[k] = v
					}
					c["destination"] = "session"
					perms = append(perms, c)
				}
			}
			resp["updatedPermissions"] = perms
		}
	} else {
		resp = map[string]any{"behavior": "deny", "message": "The user declined this."}
	}
	if pr := a.proc(); pr != nil {
		_ = pr.writeJSON(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": p.ReqID, "response": resp}})
	}
}

func (a *Agent) proc() *proc {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.p
}

func claudeStation(tool string, in map[string]any) string {
	switch tool {
	case "Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch", "NotebookRead", "ToolSearch":
		return "Read"
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return "Code"
	case "Bash", "BashOutput", "Monitor":
		c, _ := in["command"].(string)
		return commandStation(c)
	}
	return "Think"
}

func claudeApply(a *Agent, p *proc, m map[string]any) bool {
	typ, _ := m["type"].(string)
	if parent, _ := m["parent_tool_use_id"].(string); parent != "" {
		return claudeSub(a, parent, m)
	}
	sub, _ := m["subtype"].(string)
	switch typ {
	case "system":
		switch sub {
		case "init":
			a.live()
			return true
		case "thinking_tokens":
			if a.State == Working && a.Activity != "Thinking" {
				a.think()
				return true
			}
		}
		return false
	case "rate_limit_event":
		claudeLimits(a.hub, m)
		return false
	case "assistant":
		msg, _ := m["message"].(map[string]any)
		claudeContext(a, msg)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			blk, _ := b.(map[string]any)
			switch blk["type"] {
			case "text":
				t, _ := blk["text"].(string)
				a.say(t)
			case "tool_use":
				name, _ := blk["name"].(string)
				id, _ := blk["id"].(string)
				input, _ := blk["input"].(map[string]any)
				sum := claudeSummary(name, input)
				a.toolStart(id, name, sum, claudeStation(name, input))
				if (name == "Task" || name == "Agent") && !a.replaying {
					if a.subs == nil {
						a.subs = map[string]*Sub{}
					}
					a.subs[id] = &Sub{ID: id, Desc: sum, seen: time.Now()}
					a.subOrder = append(a.subOrder, id)
				}
			case "thinking":
				a.think()
			}
		}
		a.live()
		return true
	case "user":
		msg, _ := m["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		changed := false
		for _, b := range blocks {
			blk, _ := b.(map[string]any)
			if blk["type"] != "tool_result" {
				continue
			}
			id, _ := blk["tool_use_id"].(string)
			text := resultText(blk["content"])
			if s, ok := a.subs[id]; ok {
				if strings.Contains(strings.ToLower(text), "background") {
					s.bg = true
					s.seen = time.Now()
				} else {
					delete(a.subs, id)
				}
			}
			isErr, _ := blk["is_error"].(bool)
			if a.toolEnd(id, text, isErr) {
				changed = true
			}
		}
		return changed
	case "result":
		claudeWindow(a, m)
		if c, ok := m["total_cost_usd"].(float64); ok {
			a.procCost = c
			a.Cost = a.costBase + c
		}
		errText := ""
		if isErr, _ := m["is_error"].(bool); isErr {
			errText, _ = m["result"].(string)
			if errText == "" {
				errText = sub
			}
		}
		a.turnEnd(errText)
		return true
	case "control_request":
		id, _ := m["request_id"].(string)
		req, _ := m["request"].(map[string]any)
		if req["subtype"] != "can_use_tool" {
			go p.writeJSON(map[string]any{"type": "control_response", "response": map[string]any{
				"subtype": "error", "request_id": id, "error": fmt.Sprintf("switchboard does not handle %v", req["subtype"])}})
			return false
		}
		name, _ := req["tool_name"].(string)
		input, _ := req["input"].(map[string]any)
		pd := &Pending{ReqID: id, Tool: name, Input: input, Summary: claudeSummary(name, input)}
		pd.Suggestions, _ = req["permission_suggestions"].([]any)
		pd.CanAlways = len(pd.Suggestions) > 0
		if name == "AskUserQuestion" {
			raw, _ := json.Marshal(input["questions"])
			_ = json.Unmarshal(raw, &pd.Questions)
		}
		a.ask(pd)
		return true
	}
	return false
}

func claudeSub(a *Agent, parent string, m map[string]any) bool {
	s := a.subs[parent]
	if s == nil {
		return false
	}
	s.seen = time.Now()
	if m["type"] != "assistant" {
		return false
	}
	msg, _ := m["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		blk, _ := b.(map[string]any)
		if blk["type"] == "tool_use" {
			name, _ := blk["name"].(string)
			input, _ := blk["input"].(map[string]any)
			s.Activity = strings.TrimSpace(name + " " + claudeSummary(name, input))
		}
	}
	return true
}

func claudeSummary(name string, in map[string]any) string {
	str := func(k string) string { s, _ := in[k].(string); return s }
	switch name {
	case "Bash":
		return oneLine(str("command"))
	case "Read", "Write", "Edit", "MultiEdit", "NotebookEdit":
		return shortPath(str("file_path") + str("notebook_path"))
	case "Grep", "Glob":
		return oneLine(str("pattern"))
	case "WebFetch":
		return str("url")
	case "WebSearch":
		return oneLine(str("query"))
	case "Task", "Agent":
		return oneLine(str("description"))
	case "Skill":
		return str("skill")
	case "AskUserQuestion":
		return ""
	}
	for _, k := range []string{"description", "command", "file_path", "path", "query", "url", "prompt"} {
		if s := str(k); s != "" {
			return oneLine(s)
		}
	}
	return ""
}

func resultText(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
					b.WriteByte('\n')
				}
			}
		}
		return b.String()
	}
	return ""
}

var slugRE = regexp.MustCompile(`[^a-zA-Z0-9]`)

func (claudeDriver) history(a *Agent) {
	home, _ := os.UserHomeDir()
	f, err := os.Open(filepath.Join(home, ".claude", "projects", slugRE.ReplaceAllString(a.Meta.Cwd, "-"), a.Meta.ID+".jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	a.replaying = true
	defer func() { a.replaying = false }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if side, _ := m["isSidechain"].(bool); side {
			continue
		}
		switch m["type"] {
		case "assistant":
			claudeApply(a, nil, m)
		case "user":
			if meta, _ := m["isMeta"].(bool); meta {
				continue
			}
			msg, _ := m["message"].(map[string]any)
			if t, ok := msg["content"].(string); ok {
				if !strings.HasPrefix(t, "<") {
					a.addEntry(Entry{Kind: EntryUser, Text: t})
				}
				continue
			}
			claudeApply(a, nil, m)
		}
	}
	a.State = Stopped
	a.Activity = ""
}
