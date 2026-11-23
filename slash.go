package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
)

type slashCmd struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Hint        string `json:"argumentHint"`
}

type claudeInit struct {
	Commands []slashCmd `json:"commands"`
	Models   []struct {
		Value string `json:"value"`
	} `json:"models"`
}

func claudeInitialize(bin, dir string) (*claudeInit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := commandContext(ctx, bin, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()
	req, _ := json.Marshal(map[string]any{"type": "control_request", "request_id": "commands", "request": map[string]any{"subtype": "initialize"}})
	if _, err := stdin.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		var m struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string     `json:"request_id"`
				Response  claudeInit `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Type == "control_response" && m.Response.RequestID == "commands" {
			return &m.Response.Response, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("claude exited without answering initialize")
}

func (h *Hub) commands(provider, dir string) []slashCmd {
	if provider != "" && provider != "claude" {
		return nil
	}
	h.mu.Lock()
	cached, ok := h.cmds[dir]
	h.mu.Unlock()
	if ok {
		return cached
	}
	info, err := claudeInitialize(h.bin("claude"), dir)
	if err != nil {
		return nil
	}
	cmds := info.Commands
	h.mu.Lock()
	if h.cmds == nil {
		h.cmds = map[string][]slashCmd{}
	}
	h.cmds[dir] = cmds
	h.mu.Unlock()
	return cmds
}

func slashToken(value string, caret int) (string, bool) {
	r := []rune(value)
	if caret < 0 || caret > len(r) {
		caret = len(r)
	}
	if caret == 0 || r[0] != '/' {
		return "", false
	}
	for _, c := range r[:caret] {
		if unicode.IsSpace(c) {
			return "", false
		}
	}
	return string(r[1:caret]), true
}

func rankCommands(cmds []slashCmd, q string) []slashCmd {
	q = strings.ToLower(q)
	type hit struct {
		c     slashCmd
		score int
	}
	var hits []hit
	for _, c := range cmds {
		if strings.HasPrefix(c.Name, "_") || strings.HasPrefix(c.Description, "(removed)") || strings.HasSuffix(c.Description, "sessions only)") {
			continue
		}
		name := strings.ToLower(c.Name)
		_, short, _ := strings.Cut(name, ":")
		score := -1
		switch {
		case q == "" || strings.HasPrefix(name, q):
			score = 0
		case short != "" && strings.HasPrefix(short, q):
			score = 1
		case strings.Contains(name, q):
			score = 2
		}
		if score >= 0 {
			hits = append(hits, hit{c, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		if a, b := cmdGroup(hits[i].c), cmdGroup(hits[j].c); a != b {
			return a < b
		}
		return hits[i].c.Name < hits[j].c.Name
	})
	var out []slashCmd
	for i := 0; i < len(hits) && i < 8; i++ {
		out = append(out, hits[i].c)
	}
	return out
}

func cmdGroup(c slashCmd) int {
	switch {
	case strings.HasSuffix(c.Description, "(user)") || strings.HasSuffix(c.Description, "(project)"):
		return 0
	case strings.Contains(c.Name, ":"):
		return 1
	}
	return 2
}
