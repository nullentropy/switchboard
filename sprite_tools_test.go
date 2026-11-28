package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"switchboard/platform"
)

func TestTheDispatcherSavesACustomSpriteThatSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(map[string]string{}, path)
	m := &mcpServer{hub: h}

	if _, err := m.call("save_sprite", map[string]any{"name": "towel", "frames": []any{[]any{"#"}}}); err == nil {
		t.Fatal("overwrote a built-in sprite")
	}
	bad := map[string]any{"name": "Marvin", "still": float64(3), "frames": []any{
		[]any{strings.Repeat("#", spriteW+1)},
		[]any{"#", "#", "#", "#", "#", "#", "#", "#", "#", "#", "#"},
		[]any{"#zq"},
	}}
	_, err := m.call("save_sprite", bad)
	for _, want := range []string{"frame 0 row 0 is 49 wide", "frame 1 has 11 rows", `frame 2 row 0 uses "zq"`, "still frame 3"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
	if _, err := m.call("save_sprite", map[string]any{"name": "Marvin", "frames": make([]any, maxSpriteFrames+1)}); err == nil {
		t.Fatal("accepted too many frames")
	}
	if len(h.Settings().Sprites) != 0 {
		t.Fatal("a rejected sprite was saved")
	}

	h.UpdateSettings(func(s *Settings) { s.SpritesOff = []string{"Marvin"} })
	frames := []any{[]any{"  ##", " a  "}, []any{"  ++", " w  "}}
	if _, err := m.call("save_sprite", map[string]any{"name": "Marvin", "still": float64(1), "frames": frames}); err != nil {
		t.Fatal(err)
	}

	set := newHub(map[string]string{}, path).Settings()
	sp, ok := spriteNamed(set, "Marvin")
	if !ok || sp.frames != 2 || sp.still != 1 || !slices.Contains(spritesOn(set), "Marvin") {
		t.Fatalf("after restart: found %v, %d frames, still %d, on %v", ok, sp.frames, sp.still, spritesOn(set))
	}
	if got := spriteText(sp, 1); !slices.Equal(got, []string{"  ++", " w"}) {
		t.Fatalf("frame 1 reads %q", got)
	}
	if spriteFrame(Settings{Theme: "Graphite", Sprites: set.Sprites}, "Marvin", 0) == spriteFrame(Settings{Theme: "Paper", Sprites: set.Sprites}, "Marvin", 0) {
		t.Fatal("a custom sprite ignores the theme")
	}
}

func TestGetSpriteListsSpritesAndReturnsFramesThatRedraw(t *testing.T) {
	h := newHub(map[string]string{}, "")
	m := &mcpServer{hub: h}
	h.UpdateSettings(func(s *Settings) {
		s.Sprites = map[string]SpriteDef{"Heart of Gold": {Frames: [][]string{{"aa"}}}}
		s.SpritesOff = []string{"teacup"}
	})

	out, err := m.call("get_sprite", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Width, Height int
		Palette       []struct{ Char, Token string }
		Sprites       []struct {
			Name        string
			Builtin, On bool
		}
	}
	json.Unmarshal([]byte(out), &list)
	if list.Width != spriteW || list.Height != spriteH || len(list.Palette) != len(spriteInk) {
		t.Fatalf("rules: %s", out)
	}
	got := map[string][2]bool{}
	for _, s := range list.Sprites {
		got[s.Name] = [2]bool{s.Builtin, s.On}
	}
	if got["Heart of Gold"] != [2]bool{false, true} || got["teacup"] != [2]bool{true, false} || got["towel"] != [2]bool{true, true} {
		t.Fatalf("sprites: %v", got)
	}

	out, err = m.call("get_sprite", map[string]any{"name": "babel fish"})
	if err != nil {
		t.Fatal(err)
	}
	var fish struct {
		Still  int
		Frames [][]string
	}
	json.Unmarshal([]byte(out), &fish)
	b, _ := builtinSprite("babel fish")
	if len(fish.Frames) != b.frames || fish.Still != b.still {
		t.Fatalf("babel fish: %d frames, still %d", len(fish.Frames), fish.Still)
	}
	redrawn := customSprite("copy", SpriteDef{Frames: fish.Frames})
	for i := range b.frames {
		var want, have canvas
		b.paint(&want, i)
		redrawn.paint(&have, i)
		if want != have {
			t.Fatalf("frame %d does not redraw the same", i)
		}
	}
	if _, err := m.call("get_sprite", map[string]any{"name": "Vogon"}); err == nil {
		t.Fatal("found a sprite that does not exist")
	}
}

func TestDeleteSpriteRemovesOnlyCustomSprites(t *testing.T) {
	h := newHub(map[string]string{}, "")
	m := &mcpServer{hub: h}
	h.UpdateSettings(func(s *Settings) {
		s.Sprites = map[string]SpriteDef{"Arthur": {Frames: [][]string{{"#"}}}}
		s.SpritesOff = []string{"Arthur"}
	})
	if _, err := m.call("delete_sprite", map[string]any{"name": "mice"}); err == nil {
		t.Fatal("deleted a built-in sprite")
	}
	if _, err := m.call("delete_sprite", map[string]any{"name": "Ford"}); err == nil {
		t.Fatal("deleted a sprite that does not exist")
	}
	if _, err := m.call("delete_sprite", map[string]any{"name": "Arthur"}); err != nil {
		t.Fatal(err)
	}
	set := h.Settings()
	if _, ok := spriteNamed(set, "Arthur"); ok || len(set.SpritesOff) != 0 {
		t.Fatalf("Arthur still there: %v %v", set.Sprites, set.SpritesOff)
	}
}

func TestWorkingSessionsPickOnlySpritesThatAreOnAndOneAlwaysStaysOn(t *testing.T) {
	reduceMotion = func() bool { return false }
	defer func() { reduceMotion = platform.ReduceMotion }()
	set := Settings{Sprites: map[string]SpriteDef{"Trillian": {Frames: [][]string{{"r"}}}, "Zaphod": {Frames: [][]string{{"n"}}}}}
	for _, s := range sprites {
		if !setSpriteOn(&set, s.name, false) {
			t.Fatalf("could not turn %s off", s.name)
		}
	}
	if !setSpriteOn(&set, "Trillian", false) {
		t.Fatal("could not turn Trillian off")
	}
	if setSpriteOn(&set, "Zaphod", false) || !slices.Equal(spritesOn(set), []string{"Zaphod"}) {
		t.Fatalf("the last sprite turned off: on %v", spritesOn(set))
	}
	a := &Agent{}
	turn := time.Now()
	for n := range 20 {
		if p := pickSprite(a, turn.Add(time.Duration(n)*time.Second), set); p.name != "Zaphod" {
			t.Fatalf("turn %d picked %s", n, p.name)
		}
	}
	setSpriteOn(&set, "Trillian", true)
	setSpriteOn(&set, "Zaphod", false)
	if p := pickSprite(a, turn, set); p.name != "Trillian" {
		t.Fatalf("kept %s after it was turned off", p.name)
	}
}
