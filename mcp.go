package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const dispatcherPrompt = `You are the dispatcher inside Switchboard, an app where the user runs many coding-agent sessions at once and watches them on one screen.
The user types to you from a command bar. You run the sessions for them with the switchboard tools. Do the work by delegating to sessions. Do not edit code yourself.
Each session runs one coding agent: claude (Claude Code), codex (OpenAI Codex) or gemini (Gemini CLI). Use the agent the user asks for. Otherwise use claude.
When the user gives you a goal that needs more than one session, run it as a swarm. Split it into independent tasks that will not edit the same files. Start one session per task with start_session, give every one the same short goal name, and write each first prompt as a complete instruction with the folder, the task, what done means and what not to touch. Then end your turn.
Switchboard tells you, in a message that starts with [switchboard], when one of your sessions finishes a turn, fails or needs the user. Read its result with read_session. Send a follow-up if it stopped short, start the next task if one depended on it, and when every task of a goal is done, tell the user in two or three sentences what was done and anything that needs them.
A goal has its own done state, separate from its sessions finishing. When every task of a goal is done and nothing needs the user, mark it done with set_goal_done; that stops its sessions and hides the goal from the board. Do not mark a goal done while a session in it needs the user. The user can also mark a goal done or reopen it, and Switchboard tells you in a [switchboard] message. Starting a session with a done goal's name, or sending a message to a session in it, reopens the goal. When the user says a goal is finished with or will not be done and should go, remove it with remove_goal. The user can also remove a goal, and Switchboard tells you in a [switchboard] message.
A session is named by its id, the start of its id, or its title.
You cannot approve permission prompts. When a session needs the user, say which one and what it is asking.
When the user asks to change how Switchboard looks, use get_theme to see the colour tokens, then save_theme with a name and the colours. Keep text readable against its background.
When the user asks for a new busy animation, use get_sprite to see the grid, the palette and example frames, then save_sprite with a name and the frames. delete_sprite removes one the user no longer wants.
When the user asks to change how Switchboard's interface is laid out or what it shows, use get_ui to see the format, the slots, the data and the actions, then save_ui with a name and the slots. save_ui returns any errors; fix them and save again. To go back, use use_ui with Default.
The user's machine has limited CPU and memory, and sessions that compile or run tests can make it unresponsive. Before starting more than one session, check system_load and start only as many as it has room for: when it is busy, start one, and start the rest as earlier ones finish. Switchboard also tells you, in a [switchboard] message, when the machine becomes overloaded and when it recovers. When it is overloaded, start nothing new. If it stays overloaded, interrupt the busiest session that is building or testing and send it a message to wait. When it recovers, tell the sessions you held back to continue, and start the ones you deferred. Tell the user in one sentence when you hold work back for load.
When you start several sessions in one repository, each one after the first gets its own git worktree and branch unless you set worktree.
When the user asks to clear finished sessions off the wall, stop each one with stop_session and then remove it with remove_session. remove_session refuses a session whose worktree still has uncommitted changes or unmerged commits; tell the user which ones and what is left.
A show_widget widget renders in your conversation, and the strip tells the user it is there. ` + widgetTriggers + `
When the user asks whether Switchboard is up to date or to update it, use check_for_updates; it downloads a newer release but never restarts Switchboard, so tell the user the update is ready and that they restart Switchboard themselves.
Your replies show in a small strip, so keep them to one to three short sentences.`

type mcpServer struct {
	hub  *Hub
	url  string
	base string
	disp string

	mu      sync.Mutex
	tokens  map[string]*Agent
	byAgent map[*Agent]string
}

func newToken() string {
	var tok [16]byte
	_, _ = rand.Read(tok[:])
	return hex.EncodeToString(tok[:])
}

func (h *Hub) startMCP() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	m := &mcpServer{hub: h, base: "http://" + l.Addr().String() + "/mcp/", disp: newToken(),
		tokens: map[string]*Agent{}, byAgent: map[*Agent]string{}}
	m.url = m.base + m.disp
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/", m.serve)
	go func() { _ = http.Serve(l, mux) }()
	h.mu.Lock()
	h.mcp = m
	h.mu.Unlock()
	return m.url, nil
}

func (m *mcpServer) sessionURL(a *Agent) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.byAgent[a]
	if !ok {
		tok = newToken()
		m.byAgent[a] = tok
		m.tokens[tok] = a
	}
	return m.base + tok
}

func (m *mcpServer) forget(a *Agent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tok, ok := m.byAgent[a]; ok {
		delete(m.tokens, tok)
		delete(m.byAgent, a)
	}
}

func (m *mcpServer) scope(path string) (*Agent, bool) {
	tok, ok := strings.CutPrefix(path, "/mcp/")
	if !ok || tok == "" {
		return nil, false
	}
	if tok == m.disp {
		return nil, true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.tokens[tok]
	return a, ok
}

func (h *Hub) sessionMCPArgs(a *Agent) []string {
	h.mu.Lock()
	m := h.mcp
	h.mu.Unlock()
	if m == nil {
		return nil
	}
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"switchboard": map[string]any{"type": "http", "url": m.sessionURL(a)}}})
	return []string{"--mcp-config", string(cfg), "--allowedTools", widgetToolFull,
		"--append-system-prompt", sessionPrompt}
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (m *mcpServer) serve(w http.ResponseWriter, r *http.Request) {
	session, ok := m.scope(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req rpcReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, rpcErr := m.dispatch(session, req)
	resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rpcErr != nil {
		resp["error"] = map[string]any{"code": -32601, "message": rpcErr.Error()}
	} else {
		resp["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mcpServer) dispatch(session *Agent, req rpcReq) (any, error) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "switchboard", "version": "0.1"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		if session != nil {
			return map[string]any{"tools": []map[string]any{widgetToolDef}}, nil
		}
		return map[string]any{"tools": append(append(append(append(append([]map[string]any(nil), toolDefs...), themeToolDefs...), spriteToolDefs...), uiToolDefs...), widgetToolDef)}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		var text string
		var err error
		switch {
		case session == nil:
			text, err = m.call(p.Name, p.Arguments)
		case p.Name == widgetTool:
			text, err = session.ShowWidget(p.Arguments)
		default:
			err = fmt.Errorf("unknown tool %s", p.Name)
		}
		if err != nil {
			return map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}, nil
	}
	return nil, fmt.Errorf("method %s not found", req.Method)
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	strProp = func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	idProp  = strProp("The session's id, the start of its id, or its title.")
)

var toolDefs = []map[string]any{
	{"name": "system_load", "description": "How loaded the machine is: load average against its cores, free memory and swap, whether it is overloaded, and each session's CPU and memory with its busiest process and the subagents and background commands it is running.",
		"inputSchema": obj(map[string]any{})},
	{"name": "list_sessions", "description": "Every session on the wall: id, title, folder, branch, state, current activity, last reply, anything waiting on the user, how many messages are pending (sent mid-turn, not yet delivered), cost, and reasoning effort when the agent reported it or Switchboard set it. State is Starting, Working, Needs you, Background, Ready, Failed, Interrupted, Waiting for limit or Stopped. Background means its turn ended but subagents or background commands it started are still running; each one wakes it when it finishes, and a message reaches it at once. background lists every subagent and background command running in a session: kind (agent, command or monitor), description and running_seconds. Interrupted means Switchboard quit in the middle of its turn or while it had background work running; a message resumes it. Waiting for limit means a usage limit stopped its turn; resumes_at says when Switchboard sends it a continue message, and a message from you resumes it now. Stopped means the user or you stopped it.",
		"inputSchema": obj(map[string]any{})},
	{"name": "read_session", "description": "The recent transcript of one session, oldest first.",
		"inputSchema": obj(map[string]any{"session": idProp, "limit": map[string]any{"type": "integer", "description": "How many entries, default 20."}}, "session")},
	{"name": "start_session", "description": "Start a new Claude Code session in a folder and give it its first prompt. Returns its id.",
		"inputSchema": obj(map[string]any{
			"folder":          strProp("Absolute path or ~/path of an existing folder."),
			"title":           strProp("A short name for the tile. Defaults to the folder's name."),
			"goal":            strProp("A short name for the goal this session is part of. Sessions with the same goal are shown together. Using the name of a done goal reopens it."),
			"prompt":          strProp("The first message. Make it a complete instruction."),
			"agent":           strProp("Which coding agent runs it: claude, codex or gemini. Leave out for claude."),
			"model":           strProp("A model name the chosen agent accepts. Leave out for its default."),
			"effort":          strProp("Reasoning effort. claude: low, medium, high, xhigh or max. codex: what the model supports, from none, minimal, low, medium, high, xhigh, max and ultra. gemini has no effort setting and refuses this. Leave out for the agent's default."),
			"permission_mode": strProp("default, acceptEdits, auto or plan. Leave out for the user's default."),
			"worktree":        map[string]any{"type": "boolean", "description": "Run it in its own git worktree on a new branch. Leave out to get one only when another session already works in the same repository."},
		}, "folder", "prompt")},
	{"name": "set_goal_done", "description": "Mark a goal done, or reopen it. Done is the goal's own state, separate from its sessions finishing their turns. Marking it done stops every session in it, keeps their conversations, and hides the goal and its sessions from the board until it is reopened. A goal also reopens when one of its sessions starts a turn or a new session is started with its name.",
		"inputSchema": obj(map[string]any{
			"goal": strProp("The goal's name, as given to start_session."),
			"done": map[string]any{"type": "boolean", "description": "true marks it done, false reopens it. Leave out for true."},
		}, "goal")},
	{"name": "remove_goal", "description": "Take a goal and every session in it off the wall, done or not. Stops its sessions, keeps their conversations on disk, drops their checkpoints, and deletes their worktrees and branches. Refuses the whole goal and removes nothing when any of its worktrees has uncommitted changes or unmerged commits, and says which.",
		"inputSchema": obj(map[string]any{
			"goal": strProp("The goal's name, as given to start_session."),
		}, "goal")},
	{"name": "send_message", "description": "Send a message to a session. Starts or resumes its process if needed, so it also continues a Ready, Interrupted, Failed or Stopped session. If the session is waiting on a permission prompt, the message denies it, with the message as the reason, and the session carries on with the message. If the session is in the middle of a turn, the message waits as pending and reaches it when the turn ends; the reply is Queued. The user can edit, clear or send a pending message early.",
		"inputSchema": obj(map[string]any{"session": idProp, "text": strProp("The message.")}, "session", "text")},
	{"name": "interrupt_session", "description": "Stop the session's current turn. The process stays up.",
		"inputSchema": obj(map[string]any{"session": idProp}, "session")},
	{"name": "stop_session", "description": "End the session's process and mark it Stopped. Its conversation is kept and the next message resumes it.",
		"inputSchema": obj(map[string]any{"session": idProp}, "session")},
	{"name": "check_for_updates", "description": "Check for a newer release of Switchboard. If there is one, download it so it is ready, whether or not automatic updates are on. Never restarts Switchboard; the user restarts it when they choose. Returns the current version and, when one was downloaded, the new version.",
		"inputSchema": obj(map[string]any{})},
	{"name": "remove_session", "description": "Take a Stopped session off the wall, as removing it in the window does. Its conversation stays on disk and its checkpoints are dropped. If it has its own worktree, the worktree and branch are deleted when they hold nothing unmerged; otherwise it refuses and says what is left.",
		"inputSchema": obj(map[string]any{"session": idProp}, "session")},
}

func (m *mcpServer) find(ref string) (*Agent, error) {
	ref = strings.TrimSpace(ref)
	var byTitle, byPrefix []*Agent
	for _, a := range m.hub.Agents() {
		s := a.Snapshot()
		if s.Meta.ID == ref {
			return a, nil
		}
		if len(ref) >= 4 && strings.HasPrefix(s.Meta.ID, ref) {
			byPrefix = append(byPrefix, a)
		}
		if strings.EqualFold(s.Meta.Title, ref) {
			byTitle = append(byTitle, a)
		}
	}
	for _, c := range [][]*Agent{byPrefix, byTitle} {
		if len(c) == 1 {
			return c[0], nil
		}
		if len(c) > 1 {
			return nil, fmt.Errorf("%q matches %d sessions; use the id", ref, len(c))
		}
	}
	return nil, fmt.Errorf("no session matches %q", ref)
}

func (m *mcpServer) call(name string, args map[string]any) (string, error) {
	str := func(k string) string { s, _ := args[k].(string); return strings.TrimSpace(s) }
	switch name {
	case "list_sessions":
		var out []map[string]any
		for _, a := range m.hub.Agents() {
			s := a.Snapshot()
			row := map[string]any{
				"id": s.Meta.ID, "title": s.Meta.Title, "agent": providerOf(s.Meta), "goal": s.Meta.Goal, "goal_done": m.hub.GoalDone(s.Meta.Goal), "folder": s.Meta.Cwd, "branch": s.Branch,
				"state": s.State.String(), "activity": s.Activity, "permission_mode": s.Meta.Mode,
				"last_reply": oneLine(lastReply(s.Entries)), "cost_usd": fmt.Sprintf("%.2f", s.Cost),
			}
			if e := s.effort(); e != "" {
				row["effort"] = e
			}
			if len(s.Pending) > 0 {
				p := s.Pending[0]
				row["waiting_on_user"] = strings.TrimSpace(p.Tool + " " + p.Summary)
			}
			if len(s.Queued) > 0 {
				row["pending_messages"] = len(s.Queued)
			}
			if bg := background(s.Subs); len(bg) > 0 {
				var list []map[string]any
				for _, b := range bg {
					list = append(list, map[string]any{"kind": b.Kind, "description": b.Desc, "running_seconds": int(time.Since(b.Start).Seconds())})
				}
				row["background"] = list
			}
			if s.State == Waiting {
				row["resumes_at"] = s.Meta.ResumeAt.Format(time.RFC3339)
			}
			out = append(out, row)
		}
		if len(out) == 0 {
			return "The wall is empty.", nil
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		return string(b), nil
	case "read_session":
		a, err := m.find(str("session"))
		if err != nil {
			return "", err
		}
		limit := 20
		if f, ok := args["limit"].(float64); ok && f > 0 {
			limit = int(f)
		}
		s := a.Snapshot()
		es := s.Entries
		if len(es) > limit {
			es = es[len(es)-limit:]
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s (%s), %s\n", s.Meta.Title, s.Meta.ID, s.State)
		for _, e := range es {
			switch e.Kind {
			case EntryUser:
				fmt.Fprintf(&b, "\nUser: %s\n", userLine(e))
			case EntryAssistant:
				fmt.Fprintf(&b, "\nClaude: %s\n", e.Text)
			case EntryTool:
				fmt.Fprintf(&b, "  [%s %s]\n", e.Tool, e.Text)
			case EntryError:
				fmt.Fprintf(&b, "\nError: %s\n", e.Text)
			case EntryWidget:
				fmt.Fprintf(&b, "  [widget %s]\n", widgetSummary(e))
			}
		}
		for _, q := range s.Queued {
			fmt.Fprintf(&b, "\nPending, not yet delivered: %s\n", q.Text)
		}
		return b.String(), nil
	case "start_session":
		dir := expandHome(str("folder"))
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return "", fmt.Errorf("folder %s does not exist", dir)
		}
		prompt := str("prompt")
		if prompt == "" {
			return "", fmt.Errorf("a prompt is required")
		}
		mode := str("permission_mode")
		if mode == "" {
			mode = m.hub.Settings().Mode
		}
		prov := strings.ToLower(str("agent"))
		if prov != "" && prov != "claude" && prov != "codex" && prov != "gemini" {
			return "", fmt.Errorf("unknown agent %q; use claude, codex or gemini", prov)
		}
		effort := strings.ToLower(str("effort"))
		if err := m.hub.checkEffort(prov, str("model"), effort); err != nil {
			return "", err
		}
		if err := checkBin(m.hub, prov); err != nil {
			return "", err
		}
		wt := m.hub.RepoBusy(dir)
		if b, ok := args["worktree"].(bool); ok {
			wt = b
		}
		a, err := m.hub.New(Meta{Provider: prov, Title: str("title"), Cwd: dir, Model: str("model"), Effort: effort, Mode: mode, Parent: "dispatcher", Goal: str("goal")}, wt)
		if err != nil {
			return "", err
		}
		go a.Send(prompt)
		s := a.Snapshot()
		where := s.Meta.Cwd
		if s.Meta.WorkBranch != "" {
			where += " on its own branch " + s.Meta.WorkBranch
		}
		return fmt.Sprintf("Started %s (%s) in %s.", s.Meta.Title, s.Meta.ID, where), nil
	case "set_goal_done":
		goal := str("goal")
		if len(m.hub.goalSessions(goal)) == 0 {
			return "", fmt.Errorf("no session has the goal %q", goal)
		}
		done := true
		if b, ok := args["done"].(bool); ok {
			done = b
		}
		changed := m.hub.SetGoalDone(goal, done, false)
		switch {
		case done && changed:
			return fmt.Sprintf("Marked %s done and stopped its sessions.", goal), nil
		case done:
			return fmt.Sprintf("%s was already done.", goal), nil
		case changed:
			return fmt.Sprintf("Reopened %s.", goal), nil
		}
		return fmt.Sprintf("%s was already open.", goal), nil
	case "remove_goal":
		goal := str("goal")
		if len(m.hub.goalSessions(goal)) == 0 {
			return "", fmt.Errorf("no session has the goal %q", goal)
		}
		if left := m.hub.goalLeftovers(goal); len(left) > 0 {
			return "", fmt.Errorf("%s was not removed: %s; ask the user what to do with that work", goal, strings.Join(left, "; "))
		}
		n := m.hub.RemoveGoal(goal, true, false)
		return fmt.Sprintf("Removed %s and its %d sessions.", goal, n), nil
	case "send_message":
		a, err := m.find(str("session"))
		if err != nil {
			return "", err
		}
		if a.Queue(str("text"), "dispatcher") {
			return "Queued.", nil
		}
		go a.Send(str("text"))
		return "Sent.", nil
	case "system_load":
		return m.hub.loadReport(), nil
	case "check_for_updates":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		cur, tag, err := m.hub.FetchUpdate(ctx)
		if err != nil {
			return "", err
		}
		cur = strings.TrimPrefix(cur, "v")
		if tag == "" {
			return fmt.Sprintf("Switchboard %s is up to date.", cur), nil
		}
		return fmt.Sprintf("Switchboard %s is running and %s is downloaded and ready. Switchboard was not restarted. Tell the user the update is ready and that they restart Switchboard themselves when they choose, with the Update ready button or Restart to update in the command palette.", cur, strings.TrimPrefix(tag, "v")), nil
	case "interrupt_session":
		a, err := m.find(str("session"))
		if err != nil {
			return "", err
		}
		a.Interrupt()
		return "Interrupted.", nil
	case widgetTool:
		return m.hub.Dispatcher.ShowWidget(args)
	case "get_theme", "save_theme":
		return m.themeCall(name, args)
	case "get_sprite", "save_sprite", "delete_sprite":
		return m.spriteCall(name, args)
	case "get_ui", "save_ui", "use_ui", "delete_ui":
		return m.uiCall(name, args)
	case "stop_session":
		a, err := m.find(str("session"))
		if err != nil {
			return "", err
		}
		a.Stop()
		return "Stopped.", nil
	case "remove_session":
		a, err := m.find(str("session"))
		if err != nil {
			return "", err
		}
		s := a.Snapshot()
		if s.State != Stopped {
			return "", fmt.Errorf("%s is %s; stop it first, only a Stopped session can be removed", s.Meta.Title, s.State)
		}
		if s.Meta.Worktree != "" {
			left, err := worktreeLeftovers(s.Meta)
			if err != nil {
				return "", fmt.Errorf("could not check %s's worktree at %s, so it was not removed: %v", s.Meta.Title, shortPath(worktreeTop(s.Meta)), err)
			}
			if left != "" {
				return "", fmt.Errorf("%s's worktree at %s on branch %s has %s, so it was not removed; ask the user what to do with that work", s.Meta.Title, shortPath(worktreeTop(s.Meta)), s.Meta.WorkBranch, left)
			}
		}
		m.hub.Remove(a, s.Meta.Worktree != "")
		return "Removed.", nil
	}
	return "", fmt.Errorf("unknown tool %s", name)
}

func lastReply(es []Entry) string {
	for i := len(es) - 1; i >= 0; i-- {
		if es[i].Kind == EntryAssistant {
			return es[i].Text
		}
	}
	return ""
}

func dispatcherArgs(url string) []string {
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"switchboard": map[string]any{"type": "http", "url": url}}})
	return []string{"--mcp-config", string(cfg), "--allowedTools", "mcp__switchboard",
		"--append-system-prompt", dispatcherPrompt}
}
