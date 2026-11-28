package main

import (
	"testing"
	"time"

	caution "github.com/nullentropy/caution"

	"switchboard/platform"
)

func painted(s, i int) canvas {
	var c canvas
	sprites[s].paint(&c, i)
	return c
}

func TestEverySpriteHasAVisibleStillFrameAndMoves(t *testing.T) {
	for s, sp := range sprites {
		still := painted(s, sp.still)
		if still == (canvas{}) {
			t.Errorf("%s: still frame %d is empty", sp.name, sp.still)
		}
		moves := false
		for i := 1; i < sp.frames; i++ {
			if painted(s, i) != painted(s, 0) {
				moves = true
				break
			}
		}
		if !moves {
			t.Errorf("%s: every frame is the same", sp.name)
		}
	}
}

func TestSpriteFramesFollowTheTheme(t *testing.T) {
	dark := spriteFrame(Settings{Theme: "Graphite"}, "towel", 0)
	light := spriteFrame(Settings{Theme: "Paper"}, "towel", 0)
	if dark == light {
		t.Fatal("Graphite and Paper render the same frame")
	}
}

func TestSpritePickHoldsForATurnAndNeverRepeatsNext(t *testing.T) {
	reduceMotion = func() bool { return false }
	defer func() { reduceMotion = platform.ReduceMotion }()
	a := &Agent{}
	turn := time.Now()
	prev := pickSprite(a, turn, Settings{})
	if again := pickSprite(a, turn, Settings{}); again != prev {
		t.Fatalf("same turn picked %s then %s", prev.name, again.name)
	}
	for n := 1; n <= 200; n++ {
		p := pickSprite(a, turn.Add(time.Duration(n)*time.Second), Settings{})
		if p.name == prev.name {
			t.Fatalf("turn %d repeated sprite %s", n, p.name)
		}
		prev = p
	}
}

func TestBusySpriteShowsOnlyWhileWorking(t *testing.T) {
	reduceMotion = func() bool { return true }
	defer func() { reduceMotion = platform.ReduceMotion }()
	v := openView(&Agent{}, &Agent{})
	f := v.focus
	f.activity = caution.Label("")
	box := caution.Panel().Kids(f.activity)
	f.busy = newBusy(f, box)
	turn := time.Now()

	for _, st := range []State{Starting, Working} {
		f.busy.set(Snapshot{State: st, TurnFrom: turn})
		if f.busy.img.Parent() != box {
			t.Fatalf("%v: sprite not shown", st)
		}
		if f.busy.timer != nil {
			t.Fatalf("%v: reduced motion still runs a frame timer", st)
		}
		if sp, _ := spriteNamed(Settings{}, f.busy.pick.name); f.busy.frame != sp.still {
			t.Fatalf("%v: reduced motion shows frame %d, want the still frame", st, f.busy.frame)
		}
	}
	for _, st := range []State{Ready, NeedsYou, Failed, Stopped, Interrupted} {
		f.busy.set(Snapshot{State: Working, TurnFrom: turn})
		f.busy.set(Snapshot{State: st, TurnFrom: turn})
		if f.busy.img.Parent() != nil || f.busy.on {
			t.Fatalf("%v: sprite still shown", st)
		}
	}
}
