package main

import (
	"strings"
	"testing"
	"time"
)

func vancouver(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Vancouver")
	if err != nil {
		t.Skip("no tz database")
	}
	return loc
}

func TestALimitErrorGivesItsResetTime(t *testing.T) {
	van := vancouver(t)
	afternoon := time.Date(2026, 11, 23, 15, 0, 0, 0, van)
	evening := time.Date(2026, 11, 23, 19, 0, 0, 0, van)
	october := time.Date(2026, 10, 25, 9, 0, 0, 0, van)
	december := time.Date(2026, 12, 30, 9, 0, 0, 0, van)
	cases := []struct {
		name, text string
		now        time.Time
		limit      bool
		want       time.Time
	}{
		{"org spend limit with a 5-hour reset",
			"You've hit your org's monthly spend limit · run /usage-credits to ask your admin for a higher limit · your 5-hour limit resets 6:30pm (America/Vancouver)",
			afternoon, true, time.Date(2026, 11, 23, 18, 30, 0, 0, van)},
		{"hour with no minutes",
			"You've hit your org's monthly spend limit · run /usage-credits to ask your admin for a higher limit · your weekly limit resets 10pm (America/Vancouver)",
			afternoon, true, time.Date(2026, 11, 23, 22, 0, 0, 0, van)},
		{"a time already past today means tomorrow",
			"You've hit your session limit · resets 6:40pm (America/Vancouver)",
			evening, true, time.Date(2026, 11, 24, 18, 40, 0, 0, van)},
		{"a date and a time",
			"You've hit your individual spend limit · ask your admin for a higher limit · your weekly limit resets Oct 30 at 7pm (America/Vancouver)",
			october, true, time.Date(2026, 10, 30, 19, 0, 0, 0, van)},
		{"a date early next year",
			"You've hit your weekly limit · resets Jan 2 at 7am (America/Vancouver)",
			december, true, time.Date(2027, 1, 2, 7, 0, 0, 0, van)},
		{"another zone",
			"You've hit your 5-hour limit · resets 1am (UTC)",
			afternoon, true, time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC)},
		{"a limit with no time",
			"You've hit your usage limit",
			afternoon, true, time.Time{}},
		{"some other error",
			"API Error: 500 Internal server error",
			afternoon, false, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, limit := limitReset(c.text, c.now)
			if limit != c.limit || !got.Equal(c.want) {
				t.Fatalf("limitReset = %v, %v; want %v, %v", got, limit, c.want, c.limit)
			}
		})
	}
}

func TestASessionResumesAMinuteAfterTheResetOrAnHourLaterWithNoTime(t *testing.T) {
	van := vancouver(t)
	now := time.Date(2026, 11, 23, 15, 0, 0, 0, van)
	at, ok := limitResumeAt("You've hit your 5-hour limit · resets 6:30pm (America/Vancouver)", now)
	if !ok || !at.Equal(time.Date(2026, 11, 23, 18, 31, 0, 0, van)) {
		t.Fatalf("resume at %v, %v; want 18:31", at, ok)
	}
	at, ok = limitResumeAt("You've hit your usage limit", now)
	if !ok || !at.Equal(now.Add(time.Hour)) {
		t.Fatalf("resume at %v, %v; want an hour from now", at, ok)
	}
	if _, ok := limitResumeAt("The turn failed.", now); ok {
		t.Fatal("an ordinary error must not wait")
	}
}

func claudeResult(a *Agent, text string) {
	a.event(func() bool {
		return claudeApply(a, nil, map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": text})
	})
}

func TestAClaudeLimitStopWaitsAndResumesAfterTheReset(t *testing.T) {
	van := vancouver(t)
	now := time.Date(2026, 11, 23, 15, 0, 0, 0, van)
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = time.Now })

	a, d := busyAgent(t)
	claudeResult(a, "You've hit your org's monthly spend limit · run /usage-credits to ask your admin for a higher limit · your 5-hour limit resets 6:30pm (America/Vancouver)")
	s := a.Snapshot()
	want := time.Date(2026, 11, 23, 18, 31, 0, 0, van)
	if s.State != Waiting || !s.Meta.ResumeAt.Equal(want) {
		t.Fatalf("state %v resume %v; want Waiting at %v", s.State, s.Meta.ResumeAt, want)
	}
	if text, _ := statusText(s); !strings.Contains(text, "6:31pm") {
		t.Fatalf("status %q does not give the resume time", text)
	}

	if got := dueWaiting([]*Agent{a}, want.Add(-time.Second)); got != nil {
		t.Fatal("resumed before the reset")
	}
	if got := dueWaiting([]*Agent{a}, want); got != a {
		t.Fatal("not due at its resume time")
	}
	a.resumeLimit()
	sent := d.delivered()
	if s := a.Snapshot(); s.State != Working || sent[len(sent)-1] != limitNote || !s.Meta.ResumeAt.IsZero() {
		t.Fatalf("state %v, last sent %q, resume %v", s.State, sent[len(sent)-1], s.Meta.ResumeAt)
	}
}

func TestTheEarliestDueSessionResumesFirst(t *testing.T) {
	h := newHub(nil, "")
	now := time.Now()
	mk := func(st State, at time.Time) *Agent {
		a, _ := h.New(Meta{Cwd: t.TempDir(), Mode: "default"}, false)
		a.mu.Lock()
		a.State, a.Meta.ResumeAt = st, at
		a.mu.Unlock()
		return a
	}
	later := mk(Waiting, now.Add(-time.Minute))
	first := mk(Waiting, now.Add(-time.Hour))
	mk(Waiting, now.Add(time.Hour))
	mk(Ready, now.Add(-2*time.Hour))
	if got := dueWaiting([]*Agent{later, first}, now); got != first {
		t.Fatal("did not pick the earliest due session")
	}
	if got := dueWaiting(h.Agents()[2:], now); got != nil {
		t.Fatal("picked a session that is not due or not waiting")
	}
}

func TestAMessageToAWaitingSessionCancelsTheWait(t *testing.T) {
	a, d := busyAgent(t)
	claudeResult(a, "You've hit your usage limit")
	if s := a.Snapshot(); s.State != Waiting {
		t.Fatalf("state %v, want Waiting", s.State)
	}
	a.Send("Carry on with the towel inventory")
	s := a.Snapshot()
	sent := d.delivered()
	if s.State != Working || !s.Meta.ResumeAt.IsZero() || sent[len(sent)-1] != "Carry on with the towel inventory" {
		t.Fatalf("state %v, resume %v, last sent %q", s.State, s.Meta.ResumeAt, sent[len(sent)-1])
	}
}

func TestCancellingAWaitLeavesTheSessionReady(t *testing.T) {
	a, _ := busyAgent(t)
	claudeResult(a, "You've hit your usage limit")
	a.CancelWait()
	if s := a.Snapshot(); s.State != Ready || !s.Meta.ResumeAt.IsZero() {
		t.Fatalf("state %v, resume %v; want Ready with no resume time", s.State, s.Meta.ResumeAt)
	}
}

func TestAWaitingSessionIsStillWaitingAfterARestart(t *testing.T) {
	a, _ := stubAgent(t)
	setState(a, Waiting)
	if restoredState(a.lastState()) != Waiting {
		t.Fatal("Waiting does not survive a restart")
	}
}
