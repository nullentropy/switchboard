package main

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Settings struct {
	Theme      string                       `json:"theme"`
	Sounds     bool                         `json:"sounds"`
	Mode       string                       `json:"mode"`
	Model      string                       `json:"model"`
	Provider   string                       `json:"provider,omitempty"`
	WindowW    int                          `json:"windowW,omitempty"`
	WindowH    int                          `json:"windowH,omitempty"`
	Maximized  bool                         `json:"maximized,omitempty"`
	SidebarW   int                          `json:"sidebarW,omitempty"`
	LastCwd    string                       `json:"lastCwd"`
	Themes     map[string]map[string]string `json:"themes,omitempty"`
	UpdatesOff bool                         `json:"updatesOff,omitempty"`
	Sprites    map[string]SpriteDef         `json:"sprites,omitempty"`
	SpritesOff []string                     `json:"spritesOff,omitempty"`
}

type saved struct {
	Settings   Settings `json:"settings"`
	Sessions   []Meta   `json:"sessions"`
	Dispatcher *Meta    `json:"dispatcher,omitempty"`
	DoneGoals  []string `json:"doneGoals,omitempty"`
}

type Hub struct {
	bins map[string]string
	path string

	mu        sync.Mutex
	settings  Settings
	agents    []*Agent
	doneGoals map[string]bool
	views     map[*View]struct{}
	saveMu    sync.Mutex

	Dispatcher   *Agent
	reports      []string
	cmds         map[string][]slashCmd
	claudeModels []string
	limits       map[string]Limits
	quitting     atomic.Bool
	interrupted  []*Agent
	updateTag    string
	updateAsked  bool
	restart      func() error
	updater      *updater
	stageMu      sync.Mutex
	mcp          *mcpServer
}

func newHub(bins map[string]string, path string) *Hub {
	h := &Hub{bins: bins, path: path, views: map[*View]struct{}{}, doneGoals: map[string]bool{},
		settings: Settings{Theme: "Gruvbox", Sounds: true, Mode: "default"}}
	b, err := os.ReadFile(path)
	if path != "" && err == nil {
		var s saved
		if err := json.Unmarshal(b, &s); err != nil {
			log.Printf("read %s: %v", path, err)
		} else {
			h.settings = s.Settings
			for _, g := range s.DoneGoals {
				h.doneGoals[g] = true
			}
			if s.Dispatcher != nil {
				h.Dispatcher = newAgent(h, *s.Dispatcher)
				if s.Dispatcher.Launched {
					h.Dispatcher.drv.history(h.Dispatcher)
				}
			}
			for _, m := range s.Sessions {
				a := newAgent(h, m)
				if m.Launched {
					a.drv.history(a)
				}
				a.State = restoredState(m.Last)
				if a.State == Interrupted {
					h.interrupted = append(h.interrupted, a)
				}
				go a.refreshBranch()
				h.agents = append(h.agents, a)
			}
		}
	}
	if h.settings.Theme == "" {
		h.settings.Theme = "Gruvbox"
	}
	if h.Dispatcher == nil {
		home, _ := os.UserHomeDir()
		h.Dispatcher = newAgent(h, Meta{ID: newID(), Title: "Dispatcher", Cwd: home, Mode: "default", Created: time.Now()})
	}
	return h
}

func (h *Hub) Agents() []*Agent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*Agent(nil), h.agents...)
}

func (h *Hub) Settings() Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settings
}

func (h *Hub) UpdateSettings(fn func(*Settings)) {
	h.mu.Lock()
	fn(&h.settings)
	h.mu.Unlock()
	h.save()
	h.each(func(v *View) { v.settingsChanged() })
}

func (h *Hub) SetDispatcherModel(model string) {
	d := h.Dispatcher
	d.mu.Lock()
	d.Meta.Model = model
	d.mu.Unlock()
	h.save()
	d.changed()
}

func (h *Hub) dispatcherModelChoices() ([]string, int) {
	model := h.Dispatcher.Snapshot().Meta.Model
	names := append([]string{"Default"}, h.models("claude")...)
	if model != "" && indexOf(names, model) == 0 && names[0] != model {
		names = append(names, model)
	}
	return names, indexOf(names, model)
}

func (h *Hub) RepoBusy(dir string) bool {
	root := repoRoot(dir)
	if root == "" {
		return false
	}
	for _, a := range h.Agents() {
		if a.Snapshot().Meta.Repo == root {
			return true
		}
	}
	return false
}

func (h *Hub) New(m Meta, worktree bool) (*Agent, error) {
	if m.Title == "" {
		m.Title = filepath.Base(m.Cwd)
	}
	m.ID, m.Created = newID(), time.Now()
	if m.Provider == "claude" {
		m.Provider = ""
	}
	cwd := m.Cwd
	m.Repo = repoRoot(m.Cwd)
	if worktree && m.Repo != "" {
		dir, branch, err := addWorktree(m.Cwd, worktreeHome(), slug(m.Title)+"-"+m.ID[:4])
		if err != nil {
			return nil, err
		}
		m.Cwd, m.WorkBranch = dir, branch
		m.Worktree, _ = git(dir, nil, "rev-parse", "--show-toplevel")
	}
	a := newAgent(h, m)
	h.mu.Lock()
	h.agents = append(h.agents, a)
	h.settings.LastCwd = cwd
	h.mu.Unlock()
	h.reopenGoal(m.Goal)
	go a.refreshBranch()
	h.save()
	h.each(func(v *View) { v.agentAdded(a) })
	return a, nil
}

func sessionFolder(m Meta) string {
	if m.WorkBranch != "" && m.Repo != "" {
		return m.Repo
	}
	return m.Cwd
}

func (h *Hub) recentFolders(first string) []string {
	var metas []Meta
	for _, a := range h.Agents() {
		metas = append(metas, a.Snapshot().Meta)
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].Created.After(metas[j].Created) })
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(first)
	add(h.Settings().LastCwd)
	for _, m := range metas {
		add(sessionFolder(m))
	}
	return out
}

func titleFrom(prompt, dir string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return clip(line, 40)
		}
	}
	return filepath.Base(dir)
}

func (h *Hub) Remove(a *Agent, dropWorktree bool) {
	a.Stop()
	meta := a.Snapshot().Meta
	if meta.Repo != "" {
		dropCheckpoints(meta.Repo, meta.ID)
	}
	if dropWorktree {
		if err := removeWorktree(meta); err != nil {
			log.Printf("remove worktree %s: %v", meta.Worktree, err)
		}
	}
	h.mu.Lock()
	for i, x := range h.agents {
		if x == a {
			h.agents = append(h.agents[:i:i], h.agents[i+1:]...)
			break
		}
	}
	m := h.mcp
	h.mu.Unlock()
	if m != nil {
		m.forget(a)
	}
	h.save()
	h.each(func(v *View) { v.agentRemoved(a) })
}

func (h *Hub) GoalDone(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return name != "" && h.doneGoals[name]
}

func (h *Hub) goalSessions(name string) []*Agent {
	if name == "" {
		return nil
	}
	var out []*Agent
	for _, a := range h.Agents() {
		if a.Snapshot().Meta.Goal == name {
			out = append(out, a)
		}
	}
	return out
}

func (h *Hub) SetGoalDone(name string, done bool, byUser bool) bool {
	if name == "" {
		return false
	}
	h.mu.Lock()
	changed := h.doneGoals[name] != done
	if h.doneGoals == nil {
		h.doneGoals = map[string]bool{}
	}
	if done {
		h.doneGoals[name] = true
	} else {
		delete(h.doneGoals, name)
	}
	h.mu.Unlock()
	if !changed {
		return false
	}
	list := h.goalSessions(name)
	if done {
		for _, a := range list {
			a.Stop()
		}
	}
	h.save()
	h.each(func(v *View) { v.goalsChanged() })
	if byUser && slices.ContainsFunc(list, func(a *Agent) bool { return a.Snapshot().Meta.Parent == "dispatcher" }) {
		what := "marked the goal done; its sessions were stopped"
		if !done {
			what = "reopened the goal"
		}
		h.mu.Lock()
		h.reports = append(h.reports, fmt.Sprintf("%s: the user %s.", name, what))
		h.mu.Unlock()
		h.flushReports()
	}
	return true
}

func (h *Hub) goalLeftovers(name string) []string {
	var out []string
	for _, a := range h.goalSessions(name) {
		m := a.Snapshot().Meta
		if m.Worktree == "" {
			continue
		}
		left, err := worktreeLeftovers(m)
		if err != nil {
			left = "changes switchboard could not check"
		}
		if left != "" {
			out = append(out, fmt.Sprintf("%s's worktree at %s on branch %s has %s", m.Title, shortPath(worktreeTop(m)), m.WorkBranch, left))
		}
	}
	return out
}

func (h *Hub) RemoveGoal(name string, keepWork bool, byUser bool) int {
	if name == "" {
		return 0
	}
	list := h.goalSessions(name)
	dispatched := slices.ContainsFunc(list, func(a *Agent) bool { return a.Snapshot().Meta.Parent == "dispatcher" })
	for _, a := range list {
		m := a.Snapshot().Meta
		drop := m.Worktree != ""
		if drop && keepWork {
			left, err := worktreeLeftovers(m)
			drop = err == nil && left == ""
		}
		h.Remove(a, drop)
	}
	h.mu.Lock()
	delete(h.doneGoals, name)
	h.mu.Unlock()
	h.save()
	h.each(func(v *View) { v.goalsChanged() })
	if byUser && dispatched {
		h.mu.Lock()
		h.reports = append(h.reports, fmt.Sprintf("%s: the user removed the goal and its sessions.", name))
		h.mu.Unlock()
		h.flushReports()
	}
	return len(list)
}

func (h *Hub) reopenGoal(name string) {
	if h.GoalDone(name) {
		h.SetGoalDone(name, false, false)
	}
}

func (h *Hub) Rename(a *Agent, title string) {
	a.mu.Lock()
	a.Meta.Title = title
	a.mu.Unlock()
	h.save()
	a.changed()
}

// StopAll ends every process on quit. Each session keeps the state it had
// before, so the next launch shows it.
func (h *Hub) StopAll() {
	h.quitting.Store(true)
	var wg sync.WaitGroup
	for _, a := range append(h.Agents(), h.Dispatcher) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.kill()
		}()
	}
	wg.Wait()
}

// takeInterrupted returns the sessions that were mid-turn when Switchboard
// last quit, once per launch, so only one window asks about them.
func (h *Hub) takeInterrupted() []*Agent {
	h.mu.Lock()
	list := h.interrupted
	h.interrupted = nil
	h.mu.Unlock()
	var out []*Agent
	for _, a := range list {
		if a.Snapshot().State == Interrupted {
			out = append(out, a)
		}
	}
	return out
}

const resumeNote = "[switchboard] Switchboard restarted while you were in the middle of a turn, so that turn stopped. Check where things stand and carry on where you left off."

func (h *Hub) Resume(list []*Agent) {
	for _, a := range list {
		if a.Snapshot().State == Interrupted {
			go a.Send(resumeNote)
		}
	}
}

func (h *Hub) save() {
	if h.path == "" {
		return
	}
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	h.mu.Lock()
	s := saved{Settings: h.settings}
	agents := append([]*Agent(nil), h.agents...)
	done := maps.Clone(h.doneGoals)
	h.mu.Unlock()
	for _, a := range agents {
		a.mu.Lock()
		s.Sessions = append(s.Sessions, a.Meta)
		if g := a.Meta.Goal; done[g] {
			s.DoneGoals = append(s.DoneGoals, g)
			delete(done, g)
		}
		a.mu.Unlock()
	}
	sort.Strings(s.DoneGoals)
	if d := h.Dispatcher; d != nil {
		d.mu.Lock()
		m := d.Meta
		d.mu.Unlock()
		s.Dispatcher = &m
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.MkdirAll(filepath.Dir(h.path), 0o755)
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("save: %v", err)
		return
	}
	_ = os.Rename(tmp, h.path)
}

func (h *Hub) attach(v *View) {
	h.mu.Lock()
	h.views[v] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) detach(v *View) {
	h.mu.Lock()
	delete(h.views, v)
	h.mu.Unlock()
}

func (h *Hub) each(fn func(*View)) {
	h.mu.Lock()
	vs := make([]*View, 0, len(h.views))
	for v := range h.views {
		vs = append(vs, v)
	}
	h.mu.Unlock()
	for _, v := range vs {
		fn(v)
	}
}

func (h *Hub) notify(a *Agent) { h.each(func(v *View) { v.agentChanged(a) }) }
func (h *Hub) alert(a *Agent) {
	h.each(func(v *View) { v.cue(a, cueNeedsYou) })
	h.report(a, "needs the user")
}

func (h *Hub) done(a *Agent) {
	h.each(func(v *View) { v.cue(a, cueDone) })
	if a == h.Dispatcher {
		h.flushReports()
		return
	}
	s := a.Snapshot()
	switch {
	case s.State == Failed:
		h.report(a, "failed: "+oneLine(s.Err))
	case len(s.Entries) > 0 && s.Entries[len(s.Entries)-1].Kind == EntryError:
		h.report(a, "stopped with an error: "+oneLine(s.Entries[len(s.Entries)-1].Text))
	default:
		h.report(a, "finished its turn")
	}
}

func (h *Hub) report(a *Agent, what string) {
	s := a.Snapshot()
	if s.Meta.Parent != "dispatcher" {
		return
	}
	line := fmt.Sprintf("%s (%s, %s) %s.", s.Meta.Title, providerOf(s.Meta), s.Meta.ID[:8], what)
	if s.Meta.Goal != "" {
		line = s.Meta.Goal + ": " + line
	}
	h.mu.Lock()
	h.reports = append(h.reports, line)
	h.mu.Unlock()
	h.flushReports()
}

func (h *Hub) flushReports() {
	d := h.Dispatcher
	if st := d.Snapshot().State; st == Working || st == Starting || st == NeedsYou || st == Waiting {
		return
	}
	h.mu.Lock()
	rs := h.reports
	h.reports = nil
	h.mu.Unlock()
	if len(rs) > 0 {
		go d.Send("[switchboard]\n" + strings.Join(rs, "\n"))
	}
}
