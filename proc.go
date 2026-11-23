package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	wmu    sync.Mutex
	stderr *tailBuf
	done   chan struct{}

	rmu    sync.Mutex
	nextID int
	waits  map[string]chan rpcMsg
	rpcTag bool
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

var errExited = errors.New("the process exited")

func startProc(a *Agent, bin string, args []string, onLine func([]byte)) (*proc, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = a.Meta.Cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, stdin: stdin, stderr: &tailBuf{max: 4096}, done: make(chan struct{}), waits: map[string]chan rpcMsg{}}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}
	if a != a.hub.Dispatcher {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for sc.Scan() {
			onLine(sc.Bytes())
		}
		err := cmd.Wait()
		close(p.done)
		a.exited(p, err)
	}()
	return p, nil
}

func (p *proc) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}

func (p *proc) stop() {
	_ = p.stdin.Close()
	go func() {
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			_ = p.cmd.Process.Kill()
		}
	}()
}

func (p *proc) envelope(m map[string]any) map[string]any {
	if p.rpcTag {
		m["jsonrpc"] = "2.0"
	}
	return m
}

func (p *proc) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	p.rmu.Lock()
	p.nextID++
	id := p.nextID
	ch := make(chan rpcMsg, 1)
	p.waits[strconv.Itoa(id)] = ch
	p.rmu.Unlock()
	if err := p.writeJSON(p.envelope(map[string]any{"id": id, "method": method, "params": params})); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-p.done:
		return nil, fmt.Errorf("%s: %w", method, errExited)
	case <-time.After(timeout):
		return nil, fmt.Errorf("%s: no answer after %s", method, timeout)
	}
}

func (p *proc) notify(method string, params any) error {
	return p.writeJSON(p.envelope(map[string]any{"method": method, "params": params}))
}

func (p *proc) reply(id json.RawMessage, result any) error {
	return p.writeJSON(p.envelope(map[string]any{"id": id, "result": result}))
}

func (p *proc) replyError(id json.RawMessage, msg string) error {
	return p.writeJSON(p.envelope(map[string]any{"id": id, "error": rpcError{Code: -32601, Message: msg}}))
}

func (p *proc) route(line []byte) (rpcMsg, bool) {
	var m rpcMsg
	if json.Unmarshal(line, &m) != nil {
		return m, false
	}
	if m.Method == "" && len(m.ID) > 0 {
		key := string(m.ID)
		if s, err := strconv.Unquote(key); err == nil {
			key = s
		}
		p.rmu.Lock()
		ch := p.waits[key]
		delete(p.waits, key)
		p.rmu.Unlock()
		if ch != nil {
			ch <- m
		}
		return m, false
	}
	return m, m.Method != ""
}

type tailBuf struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (t *tailBuf) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, b...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(b), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
