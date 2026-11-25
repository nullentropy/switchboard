package main

import (
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/nullentropy/caution"
)

const (
	busyFrame  = 125 * time.Millisecond
	busyHidden = time.Second
	busyW      = 96
	busyH      = 20
	busyPad    = 8
)

var reduceMotion = osReduceMotion

type busyPick struct {
	turn  time.Time
	name  string
	still bool
}

var busyPicks = struct {
	sync.Mutex
	m map[*Agent]busyPick
}{m: map[*Agent]busyPick{}}

func pickSprite(a *Agent, turn time.Time, set Settings) busyPick {
	busyPicks.Lock()
	defer busyPicks.Unlock()
	pool := spritePool(set)
	prev, ok := busyPicks.m[a]
	if ok && prev.turn.Equal(turn) && slices.Contains(pool, prev.name) {
		return prev
	}
	if ok && len(pool) > 1 {
		pool = slices.DeleteFunc(slices.Clone(pool), func(n string) bool { return n == prev.name })
	}
	p := busyPick{turn: turn, name: pool[rand.IntN(len(pool))], still: reduceMotion()}
	busyPicks.m[a] = p
	return p
}

type busyAnim struct {
	f     *focusView
	box   *caution.Node
	img   *caution.Node
	on    bool
	pick  busyPick
	frame int
	gen   int
	timer *time.Timer
}

func newBusy(f *focusView, box *caution.Node) *busyAnim {
	return &busyAnim{f: f, box: box,
		img: caution.Image("").W(busyW).H(busyH).Anchor(caution.A{Left: px(0), CenterY: px(0)})}
}

func (b *busyAnim) set(s Snapshot) {
	if b == nil {
		return
	}
	if s.State != Working && s.State != Starting {
		b.stop()
		return
	}
	set := b.f.v.set
	p := pickSprite(b.f.a, s.TurnFrom, set)
	if b.on && p == b.pick {
		b.show()
		return
	}
	b.pick, b.frame = p, 0
	sp, _ := spriteNamed(set, p.name)
	if p.still {
		b.frame = sp.still
	}
	if !b.on {
		b.on = true
		b.box.Add(b.img)
		b.f.activity.Anchor(caution.A{Left: px(busyW + busyPad), Right: px(0), CenterY: px(0)})
	}
	if b.f.v.s != nil && !p.still {
		urls := make([]string, sp.frames)
		for i := range urls {
			urls[i] = spriteFrame(set, p.name, i)
		}
		b.f.v.s.Preload(urls...)
	}
	b.show()
	b.restart()
}

func (b *busyAnim) show() {
	b.img.SetSrc(spriteFrame(b.f.v.set, b.pick.name, b.frame))
}

func (b *busyAnim) restart() {
	b.gen++
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	if !b.pick.still && b.f.v.s != nil {
		b.after(busyFrame)
	}
}

func (b *busyAnim) after(d time.Duration) {
	gen, s := b.gen, b.f.v.s
	b.timer = time.AfterFunc(d, func() { s.Update(func() { b.step(gen) }) })
}

func (b *busyAnim) step(gen int) {
	if gen != b.gen || !b.on {
		return
	}
	if b.f.v.focus != b.f {
		b.stop()
		return
	}
	if !b.f.v.s.Visible() {
		b.after(busyHidden)
		return
	}
	sp, _ := spriteNamed(b.f.v.set, b.pick.name)
	b.frame = (b.frame + 1) % sp.frames
	b.show()
	b.after(busyFrame)
}

func (b *busyAnim) stop() {
	if b == nil || !b.on {
		return
	}
	b.on = false
	b.gen++
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.img.Remove()
	b.f.activity.Anchor(caution.A{Left: px(0), Right: px(0), CenterY: px(0)})
}
