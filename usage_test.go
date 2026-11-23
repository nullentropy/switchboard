package main

import (
	"encoding/json"
	"testing"
	"time"
)

func msg(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEachProvidersLimitsAndASessionsContextAreReadFromWhatItReports(t *testing.T) {
	h := newHub(map[string]string{}, "")
	a := newAgent(h, Meta{ID: newID(), Title: "Arthur Dent"})
	claudeApply(a, nil, msg(t, `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{
		"five_hour":{"utilization":0.29,"resetsAt":1795467600},"seven_day":{"utilization":0.11,"resetsAt":1795842000}}}}`))
	claudeApply(a, nil, msg(t, `{"type":"assistant","message":{"content":[],"usage":{"input_tokens":2,"cache_creation_input_tokens":14000,"cache_read_input_tokens":36000}}}`))
	claudeApply(a, nil, msg(t, `{"type":"result","subtype":"success","modelUsage":{
		"claude-haiku-5-5":{"inputTokens":10,"cacheReadInputTokens":100,"contextWindow":200000},
		"claude-opus-5-5":{"inputTokens":40,"cacheReadInputTokens":90000,"contextWindow":1000000}}}`))
	l := h.Limits()["claude"]
	if l.Five != 29 || l.Week != 11 || l.FiveResets.Unix() != 1795467600 {
		t.Fatalf("claude limits %+v", l)
	}
	if s := a.Snapshot(); s.CtxUsed != 50002 || s.CtxWindow != 1000000 {
		t.Fatalf("context %d of %d", s.CtxUsed, s.CtxWindow)
	}

	codexLimits(h, json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":40,"windowDurationMins":300,"resetsAt":1795467600},
		"secondary":{"usedPercent":7,"windowDurationMins":10080}}}`))
	codexLimits(h, json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":45,"windowDurationMins":300}}}`))
	if l := h.Limits()["codex"]; l.Five != 45 || l.Week != 7 || l.FiveResets.Unix() != 1795467600 {
		t.Fatalf("codex limits %+v", l)
	}
}

func TestALimitBarIsFilledToTheUsedFractionAndItsTipSaysTheNumberAndReset(t *testing.T) {
	now := time.Date(2026, 11, 23, 19, 23, 0, 0, time.UTC)
	for _, c := range []struct {
		window string
		used   float64
		resets time.Time
		fill   float64
		tip    string
	}{
		{"5-hour", 37, time.Date(2026, 11, 23, 21, 0, 0, 0, time.UTC), 0.37, "5-hour window 37% used, resets 21:00"},
		{"Weekly", 12, time.Date(2026, 11, 28, 5, 0, 0, 0, time.UTC), 0.12, "Weekly window 12% used, resets Sat 05:00"},
		{"5-hour", 104, time.Time{}, 1, "5-hour window 104% used"},
		{"Weekly", 0, time.Time{}, 0, "Weekly window 0% used"},
	} {
		fill, tip := limitBar(c.window, c.used, c.resets, now)
		if fill != c.fill || tip != c.tip {
			t.Errorf("%s %v: got %v %q, want %v %q", c.window, c.used, fill, tip, c.fill, c.tip)
		}
	}
}
