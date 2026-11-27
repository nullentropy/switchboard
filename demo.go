package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type demoPlan struct {
	title, folder, branch, provider, goal string
}

var demoPlans = []demoPlan{
	{"zaphod", "heart-of-gold", "fix-drive-timeout", "", "Find the Question"},
	{"trillian", "heart-of-gold", "nutrimatic-tea", "codex", "Find the Question"},
	{"fjords", "magrathea", "norway", "", ""},
	{"42", "magrathea", "drop-deep-thought", "gemini", ""},
	{"guide", "hitchhikers-guide", "main", "gemini", "Find the Question"},
	{"vogons", "vogon-fleet", "bypass-earth", "codex", ""},
	{"marvin", "vogon-fleet", "ci-retry", "", ""},
}

var demoLines = map[string][]string{
	"Think": {"Thinking"},
	"Read":  {"Read src/drive/improbability.go", "Grep \"towel\"", "Glob **/*_test.go", "Read docs/babel-fish.md"},
	"Code":  {"Edit src/drive/improbability.go", "Write internal/magrathea/fjords.go", "Edit web/guide/Entry.tsx"},
	"Test":  {"Bash go test ./drive/...", "Bash npm test -- guide", "Bash golangci-lint run"},
	"Run":   {"Bash git status", "Bash make build", "Bash docker compose up -d db"},
}

var demoExtra = []string{"tune-the-improbability-drive-and-brew-a-pot-of-tea", "towels", "babel-fish", "dolphins", "mice", "milliways", "bistromath", "agrajag", "petunias", "forty-two", "slartibartfast", "vogon-poetry", "pan-galactic"}

func (h *Hub) runDemo(n int) {
	home, _ := os.UserHomeDir()
	rng := rand.New(rand.NewSource(7))
	now := time.Now()
	h.limits = map[string]Limits{
		"claude": {Five: 37, Week: 12, HasFive: true, HasWeek: true, FiveResets: now.Add(97 * time.Minute), WeekResets: now.Add(105 * time.Hour)},
		"codex":  {Five: 74, Week: 93, HasFive: true, HasWeek: true, FiveResets: now.Add(3 * time.Hour)},
	}
	for i := range n {
		p := demoPlans[i%len(demoPlans)]
		if i >= len(demoPlans) {
			p.title = demoExtra[(i-len(demoPlans))%len(demoExtra)]
			p.goal = ""
			p.provider = []string{"", "codex", "gemini"}[i%3]
		}
		a := newAgent(h, Meta{ID: newID(), Provider: p.provider, Title: p.title,
			Cwd: filepath.Join(home, "code", p.folder), Mode: "default", Created: time.Now()})
		a.State = Ready
		a.Branch = p.branch
		a.Entries = []Entry{
			{Kind: EntryUser, Text: "The Improbability Drive cuts out after 42 seconds instead of 42 minutes. Find out why and fix it, with a test."},
			{Kind: EntryTool, Tool: "Grep", Text: "\"Duration\"", Detail: "src/drive/improbability.go:42:	Duration: 42,\nsrc/drive/improbability_test.go:18:	// Duration"},
			{Kind: EntryTool, Tool: "Read", Text: "src/drive/improbability.go", Detail: "1	package drive\n2	\n3	import \"time\""},
			{Kind: EntryAssistant, Text: "The drive's `Duration` is set in seconds but the config value is in minutes, so `42` becomes 42 seconds. I'll convert it where the config is read and add a test that checks a 42-minute jump."},
			{Kind: EntryTool, Tool: "Edit", Text: "src/drive/improbability.go", Detail: "-	Duration: cfg.JumpMinutes,\n+	Duration: cfg.JumpMinutes * 60,"},
			{Kind: EntryTool, Tool: "Bash", Text: "go test ./drive/...", Detail: "ok  	example/drive	0.042s"},
			{Kind: EntryAssistant, Text: "Fixed. Jumps now last the configured 42 minutes, and `TestJumpLastsTheConfiguredTime` covers it."},
		}
		if p.goal != "" {
			a.Meta.Goal = p.goal
		}
		if p.title == "vogons" {
			a.State = Failed
			a.Err = "Codex is not logged in. Run `codex login` in a terminal, then send again."
			h.agents = append(h.agents, a)
			continue
		}
		if i == 1 {
			a.State = NeedsYou
			a.Activity = "Wants to run Bash"
			a.TurnFrom = time.Now().Add(-95 * time.Second)
			a.Pending = []*Pending{{ReqID: newID(), Tool: "Bash", Summary: "npm install babel-fish@42", Input: map[string]any{"command": "npm install babel-fish@42"}, CanAlways: true}}
		}
		h.agents = append(h.agents, a)
		go a.demoLoop(rand.New(rand.NewSource(int64(rng.Int() + i))))
	}
}

func (a *Agent) demoLoop(rng *rand.Rand) {
	sleep := func(lo, hi int) { time.Sleep(time.Duration(lo+rng.Intn(hi-lo)) * time.Millisecond) }
	sleep(200, 3000)
	for len(a.Snapshot().Pending) > 0 {
		time.Sleep(500 * time.Millisecond)
	}
	for {
		a.mu.Lock()
		a.State = Working
		a.TurnFrom = time.Now()
		a.Entries = append(a.Entries, Entry{Kind: EntryUser, Text: "Don't panic. Carry on with the next step."})
		a.mu.Unlock()
		a.changed()
		path := []string{"Think", "Read", "Read", "Code", "Test", "Code", "Test", "Run", "Think"}
		for _, st := range path {
			a.mu.Lock()
			a.station = st
			lines := demoLines[st]
			a.Activity = lines[rng.Intn(len(lines))]
			if tool, rest, ok := strings.Cut(a.Activity, " "); ok {
				a.Entries = append(a.Entries, Entry{Kind: EntryTool, Tool: tool, Text: rest, Detail: "ok", Failed: st == "Test" && rng.Intn(5) == 0, Rev: 1})
				if e := &a.Entries[len(a.Entries)-1]; e.Failed {
					e.Detail = "--- FAIL: TestDeepThoughtAnswer (0.02s)\n    answer_test.go:41: got 41, want 42\nFAIL"
				} else if tool == "Edit" {
					e.Detail = "-	Duration: cfg.JumpMinutes,\n+	Duration: cfg.JumpMinutes * 60,"
				}
			}
			if st == "Read" && rng.Intn(3) == 0 && len(a.subs) == 0 {
				id := newID()
				a.subs = map[string]*Sub{id: {ID: id, Desc: "Consult the Guide", Kind: "agent", Activity: "Grep \"Improbability\"", Start: time.Now()}}
				a.subOrder = []string{id}
			}
			a.Cost += 0.01 + rng.Float64()*0.03
			if st == "Code" && rng.Intn(4) == 0 {
				a.Pending = []*Pending{{ReqID: newID(), Tool: "Edit", Summary: "src/drive/improbability.go"}}
				a.State = NeedsYou
				a.Activity = "Wants to run Edit"
				a.mu.Unlock()
				a.changed()
				sleep(6000, 12000)
				a.mu.Lock()
				a.Pending = nil
				a.State = Working
				a.Activity = "Thinking"
			}
			a.mu.Unlock()
			a.changed()
			sleep(1500, 4500)
		}
		a.mu.Lock()
		a.State = Ready
		a.station = ""
		a.Activity = ""
		a.subs = nil
		a.subOrder = nil
		a.Entries = append(a.Entries, Entry{Kind: EntryAssistant, Text: "Done. The change is in, the tests pass, and the answer is still 42."})
		a.mu.Unlock()
		a.changed()
		sleep(3000, 9000)
	}
}
