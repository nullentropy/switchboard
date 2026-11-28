package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"switchboard/platform"
)

const limitNote = "[switchboard] Your usage limit has reset. Carry on where you left off."

const (
	limitBuffer = time.Minute
	limitRetry  = time.Hour
	limitTick   = 30 * time.Second
)

var clock = time.Now

var (
	limitHit = regexp.MustCompile(`(?i)you['’]ve hit your [^·\n]*limit`)
	limitAt  = regexp.MustCompile(`(?i)resets\s+(?:([a-z]{3,9})\.?\s+(\d{1,2}),?\s+at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)(?:\s*\(([^)]+)\))?`)
)

func limitReset(text string, now time.Time) (time.Time, bool) {
	if !limitHit.MatchString(text) {
		return time.Time{}, false
	}
	m := limitAt.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}, true
	}
	loc := now.Location()
	if m[6] != "" {
		if l, err := time.LoadLocation(strings.TrimSpace(m[6])); err == nil {
			loc = l
		}
	}
	hour, _ := strconv.Atoi(m[3])
	minute, _ := strconv.Atoi(m[4])
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}, true
	}
	hour %= 12
	if strings.EqualFold(m[5], "pm") {
		hour += 12
	}
	local := now.In(loc)
	if m[1] == "" {
		at := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
		return at, true
	}
	mon, err := time.Parse("Jan", strings.ToUpper(m[1][:1])+strings.ToLower(m[1][1:3]))
	day, _ := strconv.Atoi(m[2])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, true
	}
	at := time.Date(local.Year(), mon.Month(), day, hour, minute, 0, 0, loc)
	if at.Before(now.AddDate(0, -6, 0)) {
		at = at.AddDate(1, 0, 0)
	}
	return at, true
}

func limitResumeAt(text string, now time.Time) (time.Time, bool) {
	at, ok := limitReset(text, now)
	switch {
	case !ok:
		return time.Time{}, false
	case at.IsZero():
		return now.Add(limitRetry), true
	}
	return at.Add(limitBuffer), true
}

func resumeLabel(at, now time.Time) string {
	at = at.In(now.Location())
	hm := strings.ToLower(at.Format("3:04PM"))
	y1, m1, d1 := at.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return hm
	}
	return at.Format("Mon Jan 2") + " " + hm
}

func (a *Agent) waitLimit(errText string, at time.Time) {
	a.addEntry(Entry{Kind: EntryError, Text: errText})
	if len(a.queue) > 0 {
		a.turnEnd("")
		return
	}
	a.State = Waiting
	a.Meta.ResumeAt = at
	a.Activity = ""
	a.Pending = nil
	a.station = ""
	a.turn = ""
	a.clearSubs()
	go a.refreshBranch()
	go a.hub.waiting(a, at)
}

func (a *Agent) resumeLimit() {
	a.mu.Lock()
	waiting := a.State == Waiting
	a.mu.Unlock()
	if waiting {
		a.SendFrom(limitNote, "")
	}
}

func (a *Agent) ResumeNow() {
	a.mu.Lock()
	waiting := a.State == Waiting
	a.mu.Unlock()
	if waiting {
		a.hub.report(a, "was resumed by the user before its usage limit reset")
		a.resumeLimit()
	}
}

func (a *Agent) CancelWait() {
	a.mu.Lock()
	waiting := a.State == Waiting
	if waiting {
		a.State = Ready
		a.Meta.ResumeAt = time.Time{}
	}
	a.mu.Unlock()
	if waiting {
		a.changed()
		a.hub.report(a, "is no longer waiting for its usage limit; the user cancelled the wait")
	}
}

func dueWaiting(agents []*Agent, now time.Time) *Agent {
	var best *Agent
	var bestAt time.Time
	for _, a := range agents {
		a.mu.Lock()
		st, at := a.State, a.Meta.ResumeAt
		a.mu.Unlock()
		if st != Waiting || at.After(now) {
			continue
		}
		if best == nil || at.Before(bestAt) {
			best, bestAt = a, at
		}
	}
	return best
}

func (h *Hub) waiting(a *Agent, at time.Time) {
	h.report(a, "hit its usage limit; Switchboard resumes it at "+resumeLabel(at, clock()))
	if a == h.Dispatcher {
		return
	}
	h.each(func(v *View) { v.cue(a, cueDone) })
}

func (h *Hub) watchLimits() {
	for range time.Tick(limitTick) {
		a := dueWaiting(append(h.Agents(), h.Dispatcher), clock())
		if a == nil || !platform.ReadLoad().Calm() {
			continue
		}
		h.report(a, "is resuming now that its usage limit has reset")
		go a.resumeLimit()
	}
}
