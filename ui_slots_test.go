package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	caution "github.com/nullentropy/caution"
)

const uiSlotsAll = `{
  "name": "Everything",
  "slots": {
    "header": {"type": "hstack", "kids": [{"type": "label", "text": "HEAD {{totals.need}} need"}]},
    "board": {"type": "vstack", "kids": [
      {"type": "label", "text": "BOARD"},
      {"type": "label", "each": "sessions", "as": "x", "text": "B {{x.title}}"}
    ]},
    "tile": {"type": "panel", "kids": [{"type": "label", "text": "TILE {{s.title}}"}]},
    "railRow": {"type": "hstack", "onClick": {"action": "open_session", "session": "{{s.id}}"},
      "kids": [{"type": "label", "text": "ROW {{s.title}}"}]},
    "empty": {"type": "label", "text": "EMPTY"}
  }
}`

const uiSlotsBroken = `{
  "name": "Broken",
  "slots": {
    "header": {"type": "sb.stateDot", "props": {"session": "nobody"}},
    "board": {"type": "sb.stateDot", "props": {"session": "nobody"}},
    "tile": {"type": "sb.stateDot", "props": {"session": "nobody"}},
    "railRow": {"type": "sb.stateDot", "props": {"session": "nobody"}},
    "empty": {"type": "sb.stateDot", "props": {"session": "nobody"}}
  }
}`

type uiSlotWorld struct {
	h            *Hub
	v            *View
	arthur, ford *Agent
}

func uiSlotFixture(t *testing.T, sessions bool, ifaces map[string]string, on string) *uiSlotWorld {
	t.Helper()
	h, now := uiFixture(t)
	w := &uiSlotWorld{h: h}
	if sessions {
		w.arthur = wallSession(t, h, Meta{Title: "Arthur", Provider: "claude", Cwd: "/earth", Goal: "Find the Question"})
		w.arthur.State, w.arthur.TurnFrom = NeedsYou, now.Add(-time.Minute)
		w.arthur.Pending = []*Pending{{ReqID: "tea", Tool: "Bash", Summary: "make tea"}}
		w.ford = wallSession(t, h, Meta{Title: "Ford", Provider: "codex", Cwd: "/betelgeuse"})
		w.ford.State = Working
	}
	h.settings.Interfaces = map[string]json.RawMessage{}
	for k, raw := range ifaces {
		h.settings.Interfaces[k] = json.RawMessage(raw)
	}
	h.settings.Interface = on
	v := &View{hub: h, set: h.Settings(), tiles: map[*Agent]*tile{}, cols: 3, stats: map[string]*caution.Node{}}
	v.wall = caution.Scroll().Kids(v.buildBoard())
	v.emptyGo = caution.VStack().Kids(caution.Label("No sessions yet"))
	v.empty = v.emptyGo
	v.field = caution.Panel().Kids(v.wall)
	v.headBrand = caution.HStack().Kids(caution.Label("Switchboard"))
	v.headRight = caution.HStack()
	v.statsRow, v.compact = caution.HStack().Kids(caution.Label("Need you")), caution.RichText()
	v.header = caution.Panel().Kids(v.headBrand, v.statsRow, v.headRight)
	v.uiBar, v.uiPark, v.rule = caution.Panel().H(0), caution.Panel(), caution.Panel()
	v.body = caution.Panel().Kids(v.field)
	v.root = caution.DockPanel().Kids(v.header, v.uiBar, v.rule, v.body)
	v.cmd = caution.TextField("")
	for _, a := range h.Agents() {
		v.addTile(a)
	}
	v.loadUI()
	v.layoutBoard()
	v.syncEmpty()
	v.refreshUI()
	w.v = v
	return w
}

func (w *uiSlotWorld) use(name string) {
	w.v.useUI(name)
	w.v.set = w.h.Settings()
	if w.v.loadUI() {
		w.v.applyUI()
	}
}

func (w *uiSlotWorld) openRail(a *Agent) {
	w.v.focus = &focusView{v: w.v, a: a, node: caution.Panel(), composer: caution.Textarea("")}
	w.v.buildShell()
	w.v.layoutRail()
}

func uiHasText(texts []string, sub string) bool {
	return slices.ContainsFunc(texts, func(s string) bool { return strings.Contains(s, sub) })
}

func uiCount(texts []string, sub string) int {
	n := 0
	for _, s := range texts {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

func TestUISlotsDefaultKeepsTheGoWindow(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Everything": uiSlotsAll}, "")
	v := w.v
	if v.ui != nil {
		t.Fatal("Default should load no interface")
	}
	if kids := v.wall.Children(); len(kids) != 1 || kids[0] != v.boardStack {
		t.Fatal("the wall should hold the Go board")
	}
	if v.headBrand.Parent() != v.header || v.statsRow.Parent() != v.header {
		t.Fatal("the Go header should show")
	}
	if v.uiBar.Prop("height") != 0.0 || len(v.uiBar.Children()) != 0 {
		t.Fatal("the notice bar should be empty and flat")
	}
	if texts := uiTexts(v.boardStack); !uiHasText(texts, "Arthur") || !uiHasText(texts, "Ford") || uiHasText(texts, "TILE") {
		t.Fatalf("board: %v", texts)
	}
}

func TestUISlotsAnInterfaceReplacesEverySlot(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Everything": uiSlotsAll}, "Everything")
	v := w.v
	head := uiTexts(v.header)
	if !uiHasText(head, "HEAD 1 need") || v.headBrand.Parent() == v.header || v.statsRow.Parent() == v.header {
		t.Fatalf("header: %v", head)
	}
	wall := uiTexts(v.wall)
	if !uiHasText(wall, "BOARD") || !uiHasText(wall, "B Arthur") || !uiHasText(wall, "B Ford") {
		t.Fatalf("board: %v", wall)
	}
	if uiCount(wall, "NEEDS YOU") != 1 || wall[0] != "NEEDS YOU" {
		t.Fatalf("a board with no sb.inbox should get the Go inbox at the top: %v", wall)
	}
	w.openRail(w.ford)
	rail := uiTexts(v.railList)
	if !uiHasText(rail, "ROW Arthur") || !uiHasText(rail, "ROW Ford") {
		t.Fatalf("rail: %v", rail)
	}
	if v.uiBar.Prop("height") != 0.0 {
		t.Fatal("no notice expected")
	}
}

func TestUISlotsATileSlotFillsTheGoBoard(t *testing.T) {
	src := `{"name": "Tiles", "slots": {"tile": {"type": "label", "text": "TILE {{s.title}}"}}}`
	w := uiSlotFixture(t, true, map[string]string{"Tiles": src}, "Tiles")
	texts := uiTexts(w.v.wall)
	if !uiHasText(texts, "TILE Ford") || !uiHasText(texts, "WORKING") {
		t.Fatalf("the Go board should hold the scripted tile: %v", texts)
	}
	if uiHasText(texts, "TILE Arthur") || !uiHasText(texts, "wants to run") {
		t.Fatalf("the inbox card stays Go: %v", texts)
	}
}

func TestUISlotsABoardWithAnInboxGetsNoSecondOne(t *testing.T) {
	src := `{"name": "Inbox", "slots": {"board": {"type": "vstack", "kids": [{"type": "sb.inbox"}, {"type": "label", "text": "MINE"}]}}}`
	w := uiSlotFixture(t, true, map[string]string{"Inbox": src}, "Inbox")
	if texts := uiTexts(w.v.wall); uiCount(texts, "NEEDS YOU") != 1 || !uiHasText(texts, "MINE") {
		t.Fatalf("board: %v", texts)
	}
}

func TestUISlotsTheEmptySlotShowsWithNoSessions(t *testing.T) {
	w := uiSlotFixture(t, false, map[string]string{"Everything": uiSlotsAll}, "Everything")
	if w.v.empty.Parent() != w.v.field {
		t.Fatal("the empty screen should be on the field")
	}
	if texts := uiTexts(w.v.field); !uiHasText(texts, "EMPTY") || uiHasText(texts, "No sessions yet") {
		t.Fatalf("empty: %v", texts)
	}
	w.use("")
	if w.v.empty != w.v.emptyGo || w.v.emptyGo.Parent() != w.v.field {
		t.Fatal("Default should put the Go empty screen back")
	}
}

func TestUISlotsAFailingSlotFallsBackWithANotice(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Broken": uiSlotsBroken}, "Broken")
	v := w.v
	if !uiHasText(uiTexts(v.uiBar), "Broken interface's header failed") || v.headBrand.Parent() != v.header {
		t.Fatalf("header: bar %v", uiTexts(v.uiBar))
	}
	wall := uiTexts(v.wall)
	if !uiHasText(wall, "interface's board failed") || !uiHasText(wall, `no session "nobody"`) || !uiHasText(wall, "Ford") || !uiHasText(wall, "NEEDS YOU") {
		t.Fatalf("board: %v", wall)
	}
	w.openRail(w.ford)
	if rail := uiTexts(v.railList); uiCount(rail, "interface's rail row failed") != 1 || !uiHasText(rail, "Ford") {
		t.Fatalf("rail: %v", rail)
	}

	tiles := uiSlotFixture(t, true, map[string]string{"Broken": `{"name": "Broken", "slots": {"tile": {"type": "sb.stateDot", "props": {"session": "nobody"}}}}`}, "Broken")
	if wall := uiTexts(tiles.v.wall); uiCount(wall, "interface's tile failed") != 1 || !uiHasText(wall, "Ford") {
		t.Fatalf("tile: %v", wall)
	}

	empty := uiSlotFixture(t, false, map[string]string{"Broken": uiSlotsBroken}, "Broken")
	if texts := uiTexts(empty.v.field); !uiHasText(texts, "interface's empty screen failed") || !uiHasText(texts, "No sessions yet") {
		t.Fatalf("empty: %v", texts)
	}
}

func TestUISlotsAnInterfaceThatDoesNotParseShowsDefaultWithANotice(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Bad": `{"name": "Bad", "slots": {"board": {"type": "marquee"}}}`}, "Bad")
	if bar := uiTexts(w.v.uiBar); !uiHasText(bar, "The Bad interface could not load") || !uiHasText(bar, "marquee") {
		t.Fatalf("bar: %v", bar)
	}
	if kids := w.v.wall.Children(); len(kids) != 1 || kids[0] != w.v.boardStack {
		t.Fatal("the Go board should show")
	}
}

func TestUISlotsSwitchingBackToDefaultRestoresTheGoWindow(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Everything": uiSlotsAll, "Broken": uiSlotsBroken}, "Everything")
	w.use("Broken")
	w.use("")
	v := w.v
	if v.ui != nil || v.headBrand.Parent() != v.header || v.statsRow.Parent() != v.header || len(v.uiPark.Children()) != 0 {
		t.Fatal("the Go header should be back")
	}
	if kids := v.wall.Children(); len(kids) != 1 || kids[0] != v.boardStack {
		t.Fatal("the Go board should be back")
	}
	if texts := uiTexts(v.wall); uiHasText(texts, "failed") || !uiHasText(texts, "Ford") {
		t.Fatalf("board: %v", texts)
	}
	if v.uiBar.Prop("height") != 0.0 || len(v.uiBar.Children()) != 0 {
		t.Fatal("the notice should be gone")
	}
}

func TestUISlotsSettingsChangesTakeEffectWhenTheSavedOneWasSkipped(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Everything": uiSlotsAll}, "Everything")
	w.h.bootUI(true)
	w.v.loadUI()
	w.v.applyUI()
	if w.v.ui != nil {
		t.Fatal("a skipped interface should not load")
	}
	w.v.useUI("Everything")
	w.v.set = w.h.Settings()
	w.v.loadUI()
	w.v.applyUI()
	if w.v.ui == nil || !uiHasText(uiTexts(w.v.header), "HEAD") {
		t.Fatal("choosing the saved interface again should show it")
	}
}

func TestUISlotsThePaletteSwitchesInterfaces(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{"Everything": uiSlotsAll, "Broken": uiSlotsBroken}, "Everything")
	var labels []string
	for _, it := range w.v.paletteItems() {
		if it.group == "Interface" {
			labels = append(labels, it.label)
		}
	}
	if !slices.Equal(labels, []string{"Use default interface", "Use Broken interface"}) {
		t.Fatalf("palette: %v", labels)
	}
	w.use("")
	labels = nil
	for _, it := range w.v.paletteItems() {
		if it.group == "Interface" {
			labels = append(labels, it.label)
		}
	}
	if !slices.Equal(labels, []string{"Use Broken interface", "Use Everything interface"}) {
		t.Fatalf("palette on Default: %v", labels)
	}
}

func TestUISlotsDangerousActionsAskFirst(t *testing.T) {
	w := uiSlotFixture(t, true, nil, "")
	v := w.v
	for name, act := range map[string]func(){
		"stop":   func() { v.uiStopSession(w.ford) },
		"remove": func() { v.uiRemoveSession(w.ford) },
		"goal":   func() { v.uiMarkGoalDone("Find the Question") },
	} {
		v.closeDialog()
		act()
		if v.dialog == nil {
			t.Fatalf("%s acted without a confirm dialog", name)
		}
	}
	if w.h.GoalDone("Find the Question") || !onWall(w.h, w.ford) {
		t.Fatal("nothing should happen before the dialog is confirmed")
	}
	v.closeDialog()
	v.uiMarkGoalDone("No such goal")
	if v.dialog != nil {
		t.Fatal("a goal that is gone should do nothing")
	}
}

func TestUISlotsElapsedBindingTicks(t *testing.T) {
	w := uiSlotFixture(t, true, map[string]string{
		"Clock": `{"name": "Clock", "slots": {"tile": {"type": "label", "text": "{{s.elapsed}}s"}}}`,
		"Still": `{"name": "Still", "slots": {"tile": {"type": "label", "text": "{{s.title}}"}}}`,
	}, "Clock")
	if !w.v.ui.tick {
		t.Fatal("a slot binding elapsed should render on the tick")
	}
	w.use("Still")
	if w.v.ui.tick {
		t.Fatal("a slot without elapsed should not render on the tick")
	}
}
