package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"time"

	caution "github.com/nullentropy/caution"
)

type Limits struct {
	Five, Week             float64
	FiveResets, WeekResets time.Time
	HasFive, HasWeek       bool
}

func (h *Hub) Limits() map[string]Limits {
	h.mu.Lock()
	defer h.mu.Unlock()
	return maps.Clone(h.limits)
}

func (h *Hub) setLimits(provider string, fn func(*Limits)) {
	h.mu.Lock()
	if h.limits == nil {
		h.limits = map[string]Limits{}
	}
	l := h.limits[provider]
	fn(&l)
	changed := l != h.limits[provider]
	h.limits[provider] = l
	h.mu.Unlock()
	if changed {
		go h.each(func(v *View) { v.limitsChanged() })
	}
}

func unixTime(v any) time.Time {
	if f, ok := v.(float64); ok && f > 0 {
		return time.Unix(int64(f), 0)
	}
	return time.Time{}
}

func claudeLimits(h *Hub, m map[string]any) {
	info, _ := m["rate_limit_info"].(map[string]any)
	wins, _ := info["unifiedWindows"].(map[string]any)
	if wins == nil {
		return
	}
	h.setLimits("claude", func(l *Limits) {
		if w, ok := wins["five_hour"].(map[string]any); ok {
			if u, ok := w["utilization"].(float64); ok {
				l.Five, l.HasFive = math.Round(u*1000)/10, true
			}
			if t := unixTime(w["resetsAt"]); !t.IsZero() {
				l.FiveResets = t
			}
		}
		if w, ok := wins["seven_day"].(map[string]any); ok {
			if u, ok := w["utilization"].(float64); ok {
				l.Week, l.HasWeek = math.Round(u*1000)/10, true
			}
			if t := unixTime(w["resetsAt"]); !t.IsZero() {
				l.WeekResets = t
			}
		}
	})
}

func claudeContext(a *Agent, msg map[string]any) {
	u, _ := msg["usage"].(map[string]any)
	if u == nil {
		return
	}
	n := 0
	for _, k := range []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"} {
		if f, ok := u[k].(float64); ok {
			n += int(f)
		}
	}
	if n > 0 {
		a.ctxUsed = n
	}
}

func claudeWindow(a *Agent, m map[string]any) {
	mu, _ := m["modelUsage"].(map[string]any)
	most := -1.0
	for _, v := range mu {
		e, _ := v.(map[string]any)
		in, _ := e["inputTokens"].(float64)
		cr, _ := e["cacheReadInputTokens"].(float64)
		cc, _ := e["cacheCreationInputTokens"].(float64)
		if w, ok := e["contextWindow"].(float64); ok && in+cr+cc > most {
			most = in + cr + cc
			a.ctxWindow = int(w)
		}
	}
}

func codexLimits(h *Hub, raw json.RawMessage) {
	type window struct {
		UsedPercent *float64 `json:"usedPercent"`
		Mins        int      `json:"windowDurationMins"`
		ResetsAt    int64    `json:"resetsAt"`
	}
	var r struct {
		RateLimits struct {
			Primary   *window `json:"primary"`
			Secondary *window `json:"secondary"`
		} `json:"rateLimits"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	h.setLimits("codex", func(l *Limits) {
		for i, w := range []*window{r.RateLimits.Primary, r.RateLimits.Secondary} {
			if w == nil || w.UsedPercent == nil {
				continue
			}
			week := w.Mins > 24*60 || (w.Mins == 0 && i == 1)
			var reset time.Time
			if w.ResetsAt > 0 {
				reset = time.Unix(w.ResetsAt, 0)
			}
			if week {
				l.Week, l.HasWeek = *w.UsedPercent, true
				if !reset.IsZero() {
					l.WeekResets = reset
				}
			} else {
				l.Five, l.HasFive = *w.UsedPercent, true
				if !reset.IsZero() {
					l.FiveResets = reset
				}
			}
		}
	})
}

func codexContext(a *Agent, params map[string]any) bool {
	tu, _ := params["tokenUsage"].(map[string]any)
	last, _ := tu["last"].(map[string]any)
	total, ok := last["totalTokens"].(float64)
	if !ok {
		return false
	}
	a.ctxUsed = int(total)
	if w, ok := tu["modelContextWindow"].(float64); ok && w > 0 {
		a.ctxWindow = int(w)
	}
	return true
}

func ctxPercent(used, window int) (int, bool) {
	if used <= 0 || window <= 0 {
		return 0, false
	}
	return min(100, used*100/window), true
}

func ctxText(used, window int) string {
	if p, ok := ctxPercent(used, window); ok {
		return fmt.Sprintf("context %d%%", p)
	}
	return ""
}

func (v *View) limitsChanged() { v.s.Update(v.drawLimits) }

func limitColor(p float64) string {
	switch {
	case p >= 90:
		return "$stFail"
	case p >= 70:
		return "$stNeed"
	}
	return "$inkDim"
}

const limitBarW = 32

func limitBar(window string, used float64, resets, now time.Time) (float64, string) {
	tip := fmt.Sprintf("%s window %.0f%% used", window, used)
	if !resets.IsZero() {
		if resets.Sub(now) < 24*time.Hour {
			tip += ", resets " + resets.Format("15:04")
		} else {
			tip += ", resets " + resets.Format("Mon 15:04")
		}
	}
	return min(1, max(0, used/100)), tip
}

func limitMeter(label, window string, used float64, resets time.Time) *caution.Node {
	fill, tip := limitBar(window, used, resets, time.Now())
	bar := caution.Panel().W(limitBarW).H(5).Radius(2.5).Bg("$edgeSoft").Tip(tip)
	if w := limitBarW * fill; w > 0 {
		bar.Add(caution.Panel().W(max(w, 2.5)).H(5).Radius(2.5).Bg(limitColor(used)).Anchor(caution.A{Left: px(0), CenterY: px(0)}))
	}
	return caution.HStack().Gap(5).Align("center").Tip(tip).Kids(
		caution.Label(label).FontSize(12).Color("$inkFaint"), bar)
}

func (v *View) drawLimits() {
	all := v.hub.Limits()
	v.limitsBox.Clear()
	for _, p := range providers {
		l, ok := all[p]
		if !ok || (!l.HasFive && !l.HasWeek) {
			continue
		}
		group := caution.HStack().Gap(10).Align("center").Kids(
			caution.Label(providerName(p)).FontSize(12).Color("$inkFaint"))
		if l.HasFive {
			group.Add(limitMeter("5h", "5-hour", l.Five, l.FiveResets))
		}
		if l.HasWeek {
			group.Add(limitMeter("7d", "Weekly", l.Week, l.WeekResets))
		}
		v.limitsBox.Add(group)
	}
}
