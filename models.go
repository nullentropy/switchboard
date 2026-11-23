package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"sync"
	"time"
)

var staticModels = map[string][]string{
	"claude": {"fable", "opus", "sonnet", "haiku"},
	"gemini": {"auto", "gemini-3.1-pro-preview", "gemini-3-flash-preview", "gemini-2.5-pro"},
}

var (
	codexModelsMu sync.Mutex
	codexModels   []string
)

func (h *Hub) models(provider string) []string {
	if provider == "codex" {
		codexModelsMu.Lock()
		defer codexModelsMu.Unlock()
		return codexModels
	}
	if provider == "" || provider == "claude" {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.claudeModels != nil {
			return h.claudeModels
		}
		return staticModels["claude"]
	}
	return staticModels[provider]
}

func (h *Hub) loadClaudeModels() {
	if checkBin(h, "claude") != nil {
		return
	}
	info, err := claudeInitialize(h.bin("claude"), "")
	if err != nil {
		return
	}
	var names []string
	for _, m := range info.Models {
		// The picker adds its own Default entry.
		if m.Value != "" && m.Value != "default" {
			names = append(names, m.Value)
		}
	}
	if names == nil {
		return
	}
	h.mu.Lock()
	h.claudeModels = names
	h.mu.Unlock()
}

func (h *Hub) loadCodexModels() {
	if checkBin(h, "codex") != nil {
		return
	}
	cmd := exec.Command(h.bin("codex"), "app-server")
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	for _, l := range []string{
		`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"switchboard","version":"0.1"}}}`,
		`{"method":"initialized"}`,
		`{"id":2,"method":"model/list","params":{}}`,
	} {
		_, _ = in.Write([]byte(l + "\n"))
	}
	done := make(chan []string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for sc.Scan() {
			var m struct {
				ID     json.RawMessage `json:"id"`
				Result struct {
					Data []struct {
						Model string `json:"model"`
					} `json:"data"`
				} `json:"result"`
			}
			if json.Unmarshal(sc.Bytes(), &m) != nil || string(m.ID) != "2" {
				continue
			}
			var names []string
			for _, d := range m.Result.Data {
				names = append(names, d.Model)
			}
			done <- names
			return
		}
		done <- nil
	}()
	select {
	case names := <-done:
		codexModelsMu.Lock()
		codexModels = names
		codexModelsMu.Unlock()
	case <-time.After(20 * time.Second):
	}
}
