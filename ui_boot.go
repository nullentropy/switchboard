package main

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	uiStartGrace   = 10 * time.Second
	uiStartsToDrop = 2
)

func (h *Hub) ActiveUI() (string, json.RawMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.uiSkip {
		return "", nil
	}
	return h.savedUILocked()
}

func (h *Hub) savedUILocked() (string, json.RawMessage) {
	name := h.settings.Interface
	raw, ok := h.settings.Interfaces[name]
	if name == "" || !ok {
		return "", nil
	}
	return name, raw
}

func (h *Hub) bootUI(skip bool) {
	h.mu.Lock()
	h.uiSkip = skip
	name, _ := h.savedUILocked()
	switch {
	case name == "" || skip:
		h.uiStarts = 0
	case h.uiStarts >= uiStartsToDrop:
		h.settings.Interface = ""
		h.uiStarts = 0
		h.reports = append(h.reports, fmt.Sprintf(
			"Switchboard ended within %d seconds of starting %d times in a row with the interface %q on, so it switched back to Default. %q is still saved.",
			int(uiStartGrace/time.Second), uiStartsToDrop, name, name))
	default:
		h.uiStarts++
	}
	h.mu.Unlock()
	h.save()
}

func (h *Hub) uiStarted() {
	h.mu.Lock()
	was := h.uiStarts
	h.uiStarts = 0
	h.mu.Unlock()
	if was != 0 {
		h.save()
	}
}
