package main

import (
	"fmt"
	"slices"
	"strings"
)

var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

var codexEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

func (h *Hub) efforts(provider, model string) []string {
	switch provider {
	case "", "claude":
		return claudeEfforts
	case "codex":
		codexModelsMu.Lock()
		defer codexModelsMu.Unlock()
		for _, m := range codexModels {
			if (m.Model == model || model == "" && m.Default) && len(m.Efforts) > 0 {
				return m.Efforts
			}
		}
		return codexEfforts
	}
	return nil
}

func (h *Hub) checkEffort(provider, model, effort string) error {
	if effort == "" {
		return nil
	}
	allowed := h.efforts(provider, model)
	if allowed == nil {
		return fmt.Errorf("%s has no reasoning effort setting Switchboard can pass: the %s CLI has no flag or ACP call for it, only modelConfigs in its settings.json. Leave effort out", providerName(provider), provider)
	}
	if slices.Contains(allowed, effort) {
		return nil
	}
	which := providerName(provider)
	if model != "" {
		which += " model " + model
	}
	return fmt.Errorf("%s does not accept effort %q; use one of %s", which, effort, strings.Join(allowed, ", "))
}

func (s Snapshot) effort() string {
	if s.ReportedEffort != "" {
		return s.ReportedEffort
	}
	return s.Meta.Effort
}

func (a *Agent) SetEffort(effort string) error {
	a.mu.Lock()
	m := a.Meta
	a.mu.Unlock()
	if effort == "" {
		return fmt.Errorf("pick an effort")
	}
	if err := a.hub.checkEffort(m.Provider, m.Model, effort); err != nil {
		return err
	}
	a.mu.Lock()
	a.Meta.Effort = effort
	a.mu.Unlock()
	go a.hub.save()
	if a.running() {
		a.drv.setEffort(a, effort)
	}
	a.changed()
	return nil
}
