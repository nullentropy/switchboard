package main

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type stubDriver struct {
	mu         sync.Mutex
	sent       []string
	imgs       [][]Image
	interrupts int
}

func (d *stubDriver) launch(a *Agent) (*proc, error) {
	return &proc{stderr: &tailBuf{max: 4096}, done: make(chan struct{})}, nil
}

func (d *stubDriver) send(a *Agent, text string, imgs []Image) error {
	d.mu.Lock()
	d.sent = append(d.sent, text)
	d.imgs = append(d.imgs, imgs)
	d.mu.Unlock()
	return nil
}

func (d *stubDriver) interrupt(a *Agent) {
	d.mu.Lock()
	d.interrupts++
	d.mu.Unlock()
	go endTurn(a)
}

func (d *stubDriver) answer(*Agent, *Pending, bool, bool, map[string]string, string) {}
func (d *stubDriver) setMode(*Agent, string)                                         {}
func (d *stubDriver) history(*Agent)                                                 {}

func (d *stubDriver) delivered() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.sent...)
}

func endTurn(a *Agent) { a.event(func() bool { a.turnEnd(""); return true }) }

func stubAgent(t *testing.T) (*Agent, *stubDriver) {
	t.Helper()
	h := newHub(nil, "")
	a, _ := h.New(Meta{Cwd: t.TempDir(), Mode: "default"}, false)
	d := &stubDriver{}
	a.drv = d
	return a, d
}

func busyAgent(t *testing.T) (*Agent, *stubDriver) {
	t.Helper()
	a, d := stubAgent(t)
	a.Send("Find the Ultimate Question")
	if s := a.Snapshot(); s.State != Working {
		t.Fatalf("after the first message the state is %s, want Working", s.State)
	}
	return a, d
}

func queuedTexts(s Snapshot) []string {
	var out []string
	for _, q := range s.Queued {
		out = append(out, q.Text)
	}
	return out
}

func userTexts(s Snapshot) []string {
	var out []string
	for _, e := range s.Entries {
		if e.Kind == EntryUser {
			out = append(out, e.Text)
		}
	}
	return out
}

func wantDelivered(t *testing.T, d *stubDriver, want ...string) {
	t.Helper()
	var got []string
	for range 200 {
		if got = d.delivered(); reflect.DeepEqual(got, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delivered %q, want %q", got, want)
}

func TestMessagesSentMidTurnWaitAsPendingAndReachTheAgentTogetherWhenTheTurnEnds(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.Send("Mind the Vogons")

	s := a.Snapshot()
	if got := queuedTexts(s); !reflect.DeepEqual(got, []string{"Bring a towel", "Mind the Vogons"}) {
		t.Fatalf("pending %q, want both mid-turn messages in order", got)
	}
	if got := userTexts(s); !reflect.DeepEqual(got, []string{"Find the Ultimate Question"}) {
		t.Fatalf("delivered transcript %q, want only the first message", got)
	}
	wantDelivered(t, d, "Find the Ultimate Question")

	endTurn(a)
	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel\n\nMind the Vogons")
	s = waitFor(t, a, "pending cleared", func(s Snapshot) bool { return len(s.Queued) == 0 && s.State == Working })
	if got := userTexts(s); !reflect.DeepEqual(got, []string{"Find the Ultimate Question", "Bring a towel", "Mind the Vogons"}) {
		t.Fatalf("transcript %q after delivery", got)
	}
}

func TestAClearedPendingMessageIsNeverDelivered(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.Send("Mind the Vogons")
	a.ClearQueued(a.Snapshot().Queued[0].ID)

	if got := queuedTexts(a.Snapshot()); !reflect.DeepEqual(got, []string{"Mind the Vogons"}) {
		t.Fatalf("pending %q after clearing the first", got)
	}
	endTurn(a)
	wantDelivered(t, d, "Find the Ultimate Question", "Mind the Vogons")
}

func TestAnEditedPendingMessageIsDeliveredWithItsNewTextAndHoldsDeliveryWhileOpen(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	id := a.Snapshot().Queued[0].ID
	if text, ok := a.BeginEdit(id); !ok || text != "Bring a towel" {
		t.Fatalf("BeginEdit gave %q %v", text, ok)
	}
	endTurn(a)
	waitFor(t, a, "turn end", func(s Snapshot) bool { return s.State == Ready })
	if got := d.delivered(); len(got) != 1 {
		t.Fatalf("delivered %q while a pending message was being edited", got)
	}

	a.EndEdit(id, "Bring two towels", true)
	wantDelivered(t, d, "Find the Ultimate Question", "Bring two towels")
}

func TestACancelledEditDeliversTheOriginalText(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	id := a.Snapshot().Queued[0].ID
	a.BeginEdit(id)
	endTurn(a)
	a.EndEdit(id, "", false)
	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel")
}

func TestSavingAnEmptyEditClearsThePendingMessage(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	id := a.Snapshot().Queued[0].ID
	a.BeginEdit(id)
	a.EndEdit(id, "  ", true)
	if s := a.Snapshot(); len(s.Queued) != 0 {
		t.Fatalf("pending %q after saving an empty edit", queuedTexts(s))
	}
	endTurn(a)
	waitFor(t, a, "turn end", func(s Snapshot) bool { return s.State == Ready })
	if got := d.delivered(); len(got) != 1 {
		t.Fatalf("delivered %q", got)
	}
}

func TestSendNowInterruptsTheTurnAndDeliversEveryPendingMessageInOrder(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.SendNow("Stop, the Vogons are here")

	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel\n\nStop, the Vogons are here")
	if d.interrupts != 1 {
		t.Fatalf("interrupted %d times, want 1", d.interrupts)
	}
}

func TestAPlainInterruptDeliversPendingMessages(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.Interrupt()
	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel")
}

func TestPendingMessagesSurviveAFailureAndGoWithTheNextMessage(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.exited(a.proc(), errors.New("Marvin unplugged it"))

	s := a.Snapshot()
	if s.State != Failed || !reflect.DeepEqual(queuedTexts(s), []string{"Bring a towel"}) {
		t.Fatalf("after a failure: state %s, pending %q", s.State, queuedTexts(s))
	}
	a.Send("Try again")
	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel\n\nTry again")
}

func TestSendNowOnAStoppedSessionStartsItWithThePendingMessages(t *testing.T) {
	a, d := busyAgent(t)
	a.Send("Bring a towel")
	a.exited(a.proc(), nil)
	a.SendNow("")
	wantDelivered(t, d, "Find the Ultimate Question", "Bring a towel")
}

func TestADispatcherMessageToABusySessionIsQueuedAndMarked(t *testing.T) {
	a, d := busyAgent(t)
	m := &mcpServer{hub: a.hub}
	out, err := m.call("send_message", map[string]any{"session": a.Snapshot().Meta.ID, "text": "Report to Zaphod"})
	if err != nil {
		t.Fatalf("send_message: %v", err)
	}
	if out != "Queued." {
		t.Fatalf("send_message said %q, want Queued.", out)
	}
	s := a.Snapshot()
	if len(s.Queued) != 1 || s.Queued[0].Text != "Report to Zaphod" || s.Queued[0].From != "dispatcher" {
		t.Fatalf("pending %+v, want one message from the dispatcher", s.Queued)
	}
	endTurn(a)
	wantDelivered(t, d, "Find the Ultimate Question", "Report to Zaphod")
}
