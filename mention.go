package main

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	caution "github.com/nullentropy/caution"
)

type pickItem struct {
	label, detail, insert string
}

type mentions struct {
	f       *focusView
	panel   *caution.Node
	list    *caution.Node
	once    sync.Once
	cmdOnce sync.Once
	mu      sync.Mutex
	files   []string
	cmds    []slashCmd
	value   string
	slash   bool
	matches []pickItem
}

func newMentions(f *focusView) *mentions {
	m := &mentions{f: f}
	m.list = caution.VStack().Gap(0).Pad(4).Align("stretch").NoFade().
		Anchor(caution.A{Left: px(0), Right: px(0), Top: px(0)})
	m.panel = caution.Panel().Dock("bottom").H(0).Bg("$panel").Clips().Kids(m.list)
	return m
}

func mentionToken(value string, caret int) (start int, query string, ok bool) {
	r := []rune(value)
	if caret < 0 || caret > len(r) {
		caret = len(r)
	}
	i := caret
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	if i == caret || r[i] != '@' {
		return 0, "", false
	}
	return i, string(r[i+1 : caret]), true
}

func rankFiles(files []string, q string, limit int) []string {
	q = strings.ToLower(q)
	type hit struct {
		p     string
		score int
	}
	var hits []hit
	for _, p := range files {
		lp := strings.ToLower(p)
		base := strings.ToLower(filepath.Base(p))
		score := -1
		switch {
		case q == "":
			score = 3
		case strings.HasPrefix(base, q):
			score = 0
		case strings.Contains(base, q):
			score = 1
		case strings.Contains(lp, q):
			score = 2
		}
		if score >= 0 {
			hits = append(hits, hit{p, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		return len(hits[i].p) < len(hits[j].p)
	})
	var out []string
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].p)
	}
	return out
}

func insertMention(value string, caret int, path string) string {
	start, _, ok := mentionToken(value, caret)
	if !ok {
		return value
	}
	r := []rune(value)
	if caret < 0 || caret > len(r) {
		caret = len(r)
	}
	return string(r[:start]) + "@" + path + " " + strings.TrimLeft(string(r[caret:]), " ")
}

func (m *mentions) refresh() {
	v := m.f.v
	v.s.Update(func() {
		if v.focus == m.f {
			m.input(m.value)
		}
	})
}

func (m *mentions) input(value string) {
	m.value = value
	if q, ok := slashToken(value, -1); ok {
		m.slash = true
		m.cmdOnce.Do(func() {
			snap := m.f.a.Snapshot()
			go func() {
				cmds := m.f.v.hub.commands(providerOf(snap.Meta), snap.Meta.Cwd)
				m.mu.Lock()
				m.cmds = cmds
				m.mu.Unlock()
				m.refresh()
			}()
		})
		m.mu.Lock()
		cmds := m.cmds
		m.mu.Unlock()
		var items []pickItem
		for _, c := range rankCommands(cmds, q) {
			items = append(items, pickItem{label: "/" + c.Name, detail: oneLine(c.Description), insert: "/" + c.Name + " "})
		}
		m.show(items)
		return
	}
	m.slash = false
	_, q, ok := mentionToken(value, -1)
	if !ok {
		m.show(nil)
		return
	}
	m.once.Do(func() {
		dir := m.f.a.Snapshot().Meta.Cwd
		go func() {
			files := projectFiles(dir)
			m.mu.Lock()
			m.files = files
			m.mu.Unlock()
			m.refresh()
		}()
	})
	m.mu.Lock()
	files := m.files
	m.mu.Unlock()
	var items []pickItem
	for _, p := range rankFiles(files, q, 8) {
		dir := filepath.Dir(p)
		if dir == "." {
			dir = ""
		}
		items = append(items, pickItem{label: filepath.Base(p), detail: dir, insert: p})
	}
	m.show(items)
}

func (m *mentions) show(items []pickItem) {
	m.matches = items
	m.list.Clear()
	if len(items) == 0 {
		m.panel.H(0)
		return
	}
	for i, it := range items {
		it := it
		row := caution.Panel().H(26).Radius(6).NoFade().Kids(
			caution.HStack().Gap(10).Align("center").Anchor(caution.A{Left: px(12), Right: px(12), CenterY: px(0)}).Kids(
				caution.Label(it.label).FontSize(13).Mono(),
				caution.Label(it.detail).FontSize(12).Color("$inkFaint").Clips().Fill(1),
			),
			caution.Glass().HoverFill("$hover").Anchor(fill()).OnPick(func(_, _ float64, _ *caution.Node) { m.pick(it, -1) }),
		)
		if i == 0 {
			row.Bg("$control")
		}
		m.list.Add(row)
	}
	m.panel.H(float64(len(items))*26 + 8)
}

func (m *mentions) pick(it pickItem, caret int) {
	var v string
	if m.slash {
		r := []rune(m.value)
		if caret < 0 || caret > len(r) {
			caret = len(r)
		}
		v = it.insert + strings.TrimLeft(string(r[caret:]), " ")
	} else {
		v = insertMention(m.value, caret, it.insert)
	}
	m.value = v
	m.f.composer.SetValueNow(v)
	m.f.composer.Focus()
	m.show(nil)
}

func (m *mentions) tab(_ bool, value string, caret int) bool {
	m.value = value
	if len(m.matches) == 0 {
		return false
	}
	if m.slash {
		if _, ok := slashToken(value, caret); !ok {
			return false
		}
	} else if _, _, ok := mentionToken(value, caret); !ok {
		return false
	}
	m.pick(m.matches[0], caret)
	return true
}
