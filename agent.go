package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type State int

const (
	Stopped State = iota
	Starting
	Working
	NeedsYou
	Ready
	Failed
	Interrupted
)

func (s State) String() string {
	return [...]string{"Stopped", "Starting", "Working", "Needs you", "Ready", "Failed", "Interrupted"}[s]
}

// lastState is how a session is shown after a restart, when its process is
// gone. A Stopped session counts only when the user or dispatcher stopped it.
func (a *Agent) lastState() string {
	switch a.State {
	case Working, Starting, Interrupted:
		return "interrupted"
	case NeedsYou, Ready:
		return "ready"
	case Failed:
		return "failed"
	case Stopped:
		if a.stopped {
			return "stopped"
		}
	}
	return a.Meta.Last
}

func restoredState(last string) State {
	switch last {
	case "interrupted":
		return Interrupted
	case "failed":
		return Failed
	case "stopped":
		return Stopped
	}
	return Ready
}

type EntryKind int

const (
	EntryUser EntryKind = iota
	EntryAssistant
	EntryTool
	EntryError
	EntryNote
)

type Entry struct {
	Kind   EntryKind
	Text   string
	Tool   string
	Detail string
	Failed bool
	Rev    int
	Start  time.Time
	End    time.Time
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	Question    string   `json:"question"`
	Header      string   `json:"header"`
	Options     []Option `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
}

type Pending struct {
	ReqID       string
	Tool        string
	Summary     string
	Input       map[string]any
	Suggestions []any
	Questions   []Question
	CanAlways   bool
	raw         any
}

type Queued struct {
	ID      int
	Text    string
	From    string
	Editing bool
}

type Meta struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider,omitempty"`
	Thread      string    `json:"thread,omitempty"`
	Title       string    `json:"title"`
	Cwd         string    `json:"cwd"`
	Model       string    `json:"model,omitempty"`
	Mode        string    `json:"mode"`
	Parent      string    `json:"parent,omitempty"`
	Goal        string    `json:"goal,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	Worktree    string    `json:"worktree,omitempty"`
	WorkBranch  string    `json:"workBranch,omitempty"`
	Checkpoints []string  `json:"checkpoints,omitempty"`
	Created     time.Time `json:"created"`
	Launched    bool      `json:"launched"`
	Last        string    `json:"last,omitempty"`
}

type driver interface {
	launch(a *Agent) (*proc, error)
	send(a *Agent, text string) error
	interrupt(a *Agent)
	answer(a *Agent, p *Pending, allow, always bool, answers map[string]string, reason string)
	setMode(a *Agent, mode string)
	history(a *Agent)
}

type Agent struct {
	hub *Hub
	drv driver

	mu       sync.Mutex
	Meta     Meta
	State    State
	Activity string
	Branch   string
	Entries  []Entry
	Pending  []*Pending
	Cost     float64
	TurnFrom time.Time
	Err      string

	queue   []Queued
	queueID int
	editing int

	extraArgs func() []string

	p         *proc
	procModel string
	toolIdx   map[string]int

	costBase, procCost float64
	ctxUsed, ctxWindow int
	replaying          bool
	station            string
	subs               map[string]*Sub
	subOrder           []string
	stopped            bool
	turn               string
	marks              []Mark
	note               string
	cleared            int
	resetting          bool
}

type Mark struct {
	At    time.Time
	Phase string
}

type Sub struct {
	ID       string
	Desc     string
	Activity string
	bg       bool
	seen     time.Time
}

type Snapshot struct {
	Meta      Meta
	State     State
	Activity  string
	Branch    string
	Entries   []Entry
	Pending   []*Pending
	Queued    []Queued
	Cost      float64
	TurnFrom  time.Time
	Err       string
	Station   string
	Subs      []Sub
	Marks     []Mark
	CtxUsed   int
	CtxWindow int
	Cleared   int
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func newAgent(h *Hub, m Meta) *Agent {
	a := &Agent{hub: h, Meta: m, State: Stopped, toolIdx: map[string]int{}}
	a.drv = driverFor(m.Provider)
	return a
}

func (a *Agent) changed() {
	a.mu.Lock()
	a.markPhase()
	save := false
	if a != a.hub.Dispatcher && !a.hub.quitting.Load() {
		if l := a.lastState(); l != a.Meta.Last {
			a.Meta.Last, save = l, true
		}
	}
	a.mu.Unlock()
	a.hub.notify(a)
	if save {
		go a.hub.save()
	}
}

func (a *Agent) markPhase() {
	ph := ""
	switch a.State {
	case NeedsYou:
		ph = "You"
	case Working, Starting:
		ph = a.station
		if ph == "" {
			ph = "Think"
		}
	}
	if n := len(a.marks); n > 0 && a.marks[n-1].Phase == ph {
		return
	}
	if len(a.marks) == 0 && ph == "" {
		return
	}
	a.marks = append(a.marks, Mark{time.Now(), ph})
	if len(a.marks) > 400 {
		a.marks = a.marks[len(a.marks)-300:]
	}
}

func (a *Agent) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Snapshot{
		Meta: a.Meta, State: a.State, Activity: a.Activity, Branch: a.Branch,
		Entries: append([]Entry(nil), a.Entries...), Pending: append([]*Pending(nil), a.Pending...),
		Cost: a.Cost, TurnFrom: a.TurnFrom, Err: a.Err, Station: a.station, Subs: a.subList(),
		Marks: append([]Mark(nil), a.marks...), CtxUsed: a.ctxUsed, CtxWindow: a.ctxWindow,
		Cleared: a.cleared, Queued: a.queued(),
	}
}

func (a *Agent) queued() []Queued {
	out := append([]Queued(nil), a.queue...)
	for i := range out {
		out[i].Editing = out[i].ID == a.editing
	}
	return out
}

func (a *Agent) clearConversation() {
	a.Entries = nil
	a.toolIdx = map[string]int{}
	a.cleared++
	a.resetting = true
}

func (a *Agent) adoptSession(id string) {
	if !a.resetting || id == "" {
		return
	}
	a.resetting = false
	if id != a.Meta.ID {
		a.Meta.ID = id
		go a.hub.save()
	}
}

func (a *Agent) addEntry(e Entry) int {
	a.Entries = append(a.Entries, e)
	return len(a.Entries) - 1
}

func (a *Agent) fail(err error) {
	a.mu.Lock()
	a.State = Failed
	a.Err = err.Error()
	a.addEntry(Entry{Kind: EntryError, Text: err.Error()})
	a.mu.Unlock()
	a.changed()
}

func (a *Agent) Send(text string) { a.SendFrom(text, "") }

func (a *Agent) busy() bool { return a.State == Working || a.State == Starting }

func (a *Agent) enqueue(text, from string) {
	a.queueID++
	a.queue = append(a.queue, Queued{ID: a.queueID, Text: text, From: from})
}

func (a *Agent) Queue(text, from string) bool {
	text = strings.TrimSpace(text)
	a.mu.Lock()
	held := text != "" && (a.busy() || a.editing != 0)
	if held {
		a.enqueue(text, from)
	}
	a.mu.Unlock()
	if held {
		a.changed()
	}
	return held
}

func (a *Agent) ClearQueued(id int) {
	a.mu.Lock()
	a.dropQueued(id)
	a.mu.Unlock()
	a.changed()
	a.flush()
}

func (a *Agent) dropQueued(id int) {
	for i, q := range a.queue {
		if q.ID == id {
			a.queue = append(a.queue[:i:i], a.queue[i+1:]...)
			break
		}
	}
	if a.editing == id {
		a.editing = 0
	}
}

func (a *Agent) BeginEdit(id int) (string, bool) {
	a.mu.Lock()
	defer a.changed()
	defer a.mu.Unlock()
	for _, q := range a.queue {
		if q.ID == id {
			a.editing = id
			return q.Text, true
		}
	}
	return "", false
}

func (a *Agent) EndEdit(id int, text string, save bool) {
	text = strings.TrimSpace(text)
	a.mu.Lock()
	if a.editing == id {
		a.editing = 0
	}
	if save && text == "" {
		a.dropQueued(id)
	}
	for i := range a.queue {
		if save && a.queue[i].ID == id {
			a.queue[i].Text = text
		}
	}
	a.mu.Unlock()
	a.changed()
	a.flush()
}

func (a *Agent) SendNow(text string) {
	text = strings.TrimSpace(text)
	a.mu.Lock()
	if text != "" {
		a.enqueue(text, "")
	}
	a.editing = 0
	live := a.busy() && a.p != nil
	a.mu.Unlock()
	a.changed()
	if live {
		a.drv.interrupt(a)
		return
	}
	a.SendFrom("", "")
}

func (a *Agent) flush() {
	a.mu.Lock()
	ready := a.State == Ready && a.editing == 0 && len(a.queue) > 0
	a.mu.Unlock()
	if ready {
		a.SendFrom("", "")
	}
}

func (a *Agent) SendFrom(text, from string) {
	text = strings.TrimSpace(text)
	a.mu.Lock()
	if a.busy() || a.editing != 0 {
		if text != "" {
			a.enqueue(text, from)
		}
		a.mu.Unlock()
		if text != "" {
			a.changed()
		}
		return
	}
	var texts []string
	for _, q := range a.queue {
		texts = append(texts, q.Text)
	}
	a.queue = nil
	if text != "" {
		texts = append(texts, text)
	}
	if len(texts) == 0 {
		a.mu.Unlock()
		return
	}
	var old *proc
	if a.p != nil && a.procModel != a.Meta.Model && a.State != NeedsYou {
		old, a.p = a.p, nil
		a.costBase += a.procCost
		a.procCost = 0
	}
	running := a.p != nil
	if !running {
		a.stopped = false
		a.procModel = a.Meta.Model
	}
	a.State = Starting
	joined := strings.Join(texts, "\n\n")
	send := joined
	if a.note != "" {
		send, a.note = a.note+"\n\n"+joined, ""
	}
	declined := a.Pending
	a.Pending = nil
	for _, t := range texts {
		a.addEntry(Entry{Kind: EntryUser, Text: t})
	}
	a.Activity = "Starting"
	a.TurnFrom = time.Now()
	a.Err = ""
	a.mu.Unlock()
	if old != nil {
		old.stop()
	}
	a.changed()
	if a != a.hub.Dispatcher {
		cp := a.takeCheckpoint()
		a.mu.Lock()
		for range texts {
			a.Meta.Checkpoints = append(a.Meta.Checkpoints, cp)
		}
		a.mu.Unlock()
		go a.hub.save()
	}
	if !running {
		p, err := a.drv.launch(a)
		if err != nil {
			a.fail(err)
			return
		}
		a.mu.Lock()
		a.p = p
		if !a.Meta.Launched {
			a.Meta.Launched = true
			go a.hub.save()
		}
		a.mu.Unlock()
	}
	for _, p := range declined {
		a.drv.answer(a, p, false, false, nil, "The user declined this. To tell you how to proceed, the user said:\n"+joined)
	}
	a.mu.Lock()
	a.State = Working
	a.Activity = "Thinking"
	a.station = "Think"
	a.mu.Unlock()
	a.changed()
	if err := a.drv.send(a, send); err != nil {
		a.fail(err)
	}
}

func (a *Agent) takeCheckpoint() string {
	a.mu.Lock()
	m := a.Meta
	a.mu.Unlock()
	if a == a.hub.Dispatcher || m.Repo == "" {
		return ""
	}
	sha, err := checkpoint(m.Cwd, checkpointRef(m.ID, len(m.Checkpoints)))
	if err != nil {
		log.Printf("checkpoint %s: %v", m.Title, err)
		return ""
	}
	return sha
}

func (a *Agent) Restore(n int, before string) error {
	a.mu.Lock()
	m, st := a.Meta, a.State
	a.mu.Unlock()
	if st == Working || st == Starting || st == NeedsYou {
		return fmt.Errorf("wait for the turn to finish, or interrupt it, before restoring files")
	}
	if n < 0 || n >= len(m.Checkpoints) || m.Checkpoints[n] == "" {
		return fmt.Errorf("there is no checkpoint for that message")
	}
	if err := restoreCheckpoint(m.Cwd, m.Checkpoints[n]); err != nil {
		return err
	}
	a.mu.Lock()
	a.note = "[switchboard] The user restored the files in this folder to how they were just before their message \"" + oneLine(before) + "\". Every change made after that point, by you or anyone else, is gone. Check the files again before relying on what you did since."
	a.addEntry(Entry{Kind: EntryNote, Text: "Files restored to before “" + oneLine(before) + "”"})
	a.mu.Unlock()
	a.changed()
	return nil
}

func (a *Agent) running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.p != nil
}

func (a *Agent) Interrupt() {
	if a.running() {
		a.drv.interrupt(a)
	}
}

func (a *Agent) Stop() {
	a.mu.Lock()
	p := a.p
	a.stopped = true
	idle := p == nil && a.State != Stopped
	if idle {
		a.State = Stopped
		a.Activity = ""
	}
	a.mu.Unlock()
	if p != nil {
		p.stop()
	}
	if idle {
		a.changed()
	}
}

func (a *Agent) kill() {
	a.mu.Lock()
	p := a.p
	a.stopped = true
	a.mu.Unlock()
	if p != nil {
		p.stop()
	}
}

func (a *Agent) SetMode(mode string) {
	a.mu.Lock()
	a.Meta.Mode = mode
	a.mu.Unlock()
	go a.hub.save()
	if a.running() {
		a.drv.setMode(a, mode)
	}
	a.changed()
}

func (a *Agent) Answer(reqID string, allow, always bool, answers map[string]string) {
	a.mu.Lock()
	var p *Pending
	for i, q := range a.Pending {
		if q.ReqID == reqID {
			p = q
			a.Pending = append(a.Pending[:i:i], a.Pending[i+1:]...)
			break
		}
	}
	if p == nil {
		a.mu.Unlock()
		return
	}
	if len(a.Pending) == 0 && a.State == NeedsYou {
		a.State = Working
		a.Activity = "Thinking"
	}
	a.mu.Unlock()
	a.changed()
	a.drv.answer(a, p, allow, always, answers, "")
}

func (a *Agent) event(fn func() bool) {
	a.mu.Lock()
	changed := fn()
	if len(a.Pending) > 0 {
		a.State = NeedsYou
		if len(a.Pending[0].Questions) > 0 {
			a.Activity = "Has a question"
		} else {
			a.Activity = strings.TrimSpace("Wants to run " + a.Pending[0].Tool)
		}
	}
	a.mu.Unlock()
	if changed {
		a.changed()
	}
}

func (a *Agent) live() {
	if a.State == Starting {
		a.State = Working
	}
}

func (a *Agent) say(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	a.addEntry(Entry{Kind: EntryAssistant, Text: text})
	a.Activity = "Writing"
	a.live()
}

func (a *Agent) think() {
	a.Activity = "Thinking"
	if !a.replaying {
		a.station = "Think"
	}
	a.live()
}

func (a *Agent) toolStart(id, name, summary, station string) {
	e := Entry{Kind: EntryTool, Tool: name, Text: summary}
	if !a.replaying {
		e.Start = time.Now()
	}
	a.toolIdx[id] = a.addEntry(e)
	a.Activity = strings.TrimSpace(name + " " + summary)
	if !a.replaying {
		a.station = station
	}
	a.live()
}

func (a *Agent) toolEnd(id, detail string, failed bool) bool {
	i, ok := a.toolIdx[id]
	if !ok {
		return false
	}
	a.Entries[i].Detail = trimLines(detail, 12, 1200)
	a.Entries[i].Failed = failed
	if !a.Entries[i].Start.IsZero() {
		a.Entries[i].End = time.Now()
	}
	a.Entries[i].Rev++
	if a.State == Working {
		a.Activity = "Thinking"
	}
	return true
}

func (a *Agent) ask(p *Pending) {
	a.Pending = append(a.Pending, p)
	go a.hub.alert(a)
}

func (a *Agent) turnEnd(errText string) {
	if errText != "" {
		a.addEntry(Entry{Kind: EntryError, Text: errText})
	}
	a.State = Ready
	a.Activity = ""
	a.Pending = nil
	a.station = ""
	a.turn = ""
	a.clearSubs()
	go a.refreshBranch()
	if a.editing == 0 && len(a.queue) > 0 {
		go a.flush()
		return
	}
	go a.hub.done(a)
}

func (a *Agent) exited(p *proc, err error) {
	a.mu.Lock()
	if a.p != p {
		a.mu.Unlock()
		return
	}
	a.p = nil
	a.station = ""
	a.turn = ""
	a.clearSubs()
	a.costBase += a.procCost
	a.procCost = 0
	a.Pending = nil
	a.Activity = ""
	if err != nil && !a.stopped {
		a.State = Failed
		a.Err = strings.TrimSpace(p.stderr.String())
		if a.Err == "" {
			a.Err = err.Error()
		}
		a.addEntry(Entry{Kind: EntryError, Text: providerName(a.Meta.Provider) + " exited: " + a.Err})
	} else {
		a.State = Stopped
	}
	a.mu.Unlock()
	a.changed()
}

func (a *Agent) subList() []Sub {
	var out []Sub
	for _, id := range a.subOrder {
		if s, ok := a.subs[id]; ok {
			if s.bg && time.Since(s.seen) > 45*time.Second {
				continue
			}
			out = append(out, *s)
		}
	}
	return out
}

func (a *Agent) clearSubs() {
	for id, s := range a.subs {
		if !s.bg {
			delete(a.subs, id)
		}
	}
}

func (a *Agent) refreshBranch() {
	a.mu.Lock()
	needRepo := a.Meta.Repo == ""
	a.mu.Unlock()
	if needRepo {
		if root := repoRoot(a.Meta.Cwd); root != "" {
			a.mu.Lock()
			a.Meta.Repo = root
			a.mu.Unlock()
		}
	}
	out, err := command("git", "-C", a.Meta.Cwd, "branch", "--show-current").Output()
	if err != nil {
		return
	}
	b := strings.TrimSpace(string(out))
	a.mu.Lock()
	same := a.Branch == b
	a.Branch = b
	a.mu.Unlock()
	if !same {
		a.changed()
	}
}

var testCmd = regexp.MustCompile(`(?i)\b(test|tests|pytest|jest|vitest|mocha|rspec|spec|go vet|lint|eslint|tsc|mypy|cargo check|cargo clippy|make check|ruff|golangci)\b`)

func commandStation(cmd string) string {
	if testCmd.MatchString(cmd) {
		return "Test"
	}
	return "Run"
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	sep := string(filepath.Separator)
	parts := strings.Split(p, sep)
	if len(parts) > 4 {
		p = "…" + sep + strings.Join(parts[len(parts)-3:], sep)
	}
	return p
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
}

func trimLines(s string, lines, chars int) string {
	s = strings.ReplaceAll(strings.TrimRight(s, "\n "), "\t", "    ")
	ls := strings.Split(s, "\n")
	cut := false
	if len(ls) > lines {
		ls, cut = ls[:lines], true
	}
	s = strings.Join(ls, "\n")
	if r := []rune(s); len(r) > chars {
		s, cut = string(r[:chars]), true
	}
	if cut {
		s += "\n…"
	}
	return s
}
