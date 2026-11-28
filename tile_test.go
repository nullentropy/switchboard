package main

import (
	"reflect"
	"testing"

	caution "github.com/nullentropy/caution"
)

func anchorAt(t *testing.T, n *caution.Node, side string) float64 {
	t.Helper()
	m, _ := n.Prop("anchors").(map[string]any)
	v, ok := m[side].(float64)
	if !ok {
		t.Fatalf("no %s anchor in %v", side, m)
	}
	return v
}

func TestAWorkingTileKeepsItsBorderVisibleOnAllFourSides(t *testing.T) {
	for _, selected := range []bool{false, true} {
		tl := newTile(&View{}, &Agent{})
		tl.update(Snapshot{State: Working, Meta: Meta{Title: "Heart of Gold", Cwd: "/magrathea"}})
		tl.setSelected(selected)
		bw := tl.node.Prop("borderWidth").(float64)
		radius := tl.node.Prop("radius").(float64)
		if tl.bar == nil {
			t.Fatal("a working tile has no progress bar")
		}
		if top := anchorAt(t, tl.bar, "top"); top < bw {
			t.Errorf("selected=%v: the bar sits at %v and covers the %vpx top border", selected, top, bw)
		}
		for _, side := range []string{"left", "right"} {
			if x := anchorAt(t, tl.bar, side); x < radius {
				t.Errorf("selected=%v: the bar's %s end at %v runs into the %vpx corner", selected, side, x, radius)
			}
		}
		if left := anchorAt(t, tl.edge, "left"); left < bw {
			t.Errorf("selected=%v: the edge strip sits at %v and covers the %vpx left border", selected, left, bw)
		}
		for _, side := range []string{"top", "bottom"} {
			if y := anchorAt(t, tl.edge, side); y < radius {
				t.Errorf("selected=%v: the edge strip's %s end at %v runs into the %vpx corner", selected, side, y, radius)
			}
		}
		if e, b := tl.edge.Prop("bg"), tl.node.Prop("bg"); e != b {
			t.Errorf("selected=%v: the edge strip is %v on a %v tile", selected, e, b)
		}
	}
}

func TestAWorkingMarkOrbitsInTheThemesWorkColourAndRetintsWithTheTheme(t *testing.T) {
	dark, light := Settings{Theme: "Gruvbox"}, Settings{Theme: "Paper"}
	tint := func(set Settings) map[string]float64 {
		r, g, b := tokenRGB(set, "stWork")
		return map[string]float64{"tintR": r, "tintG": g, "tintB": b}
	}
	m := newMark(dark, icLoader, 14, "$stWork")
	if m.orbit == nil || m.glyph.Prop("text") != "" || !reflect.DeepEqual(m.orbit.Prop("uniforms"), tint(dark)) {
		t.Fatalf("working mark: orbit %v glyph %q", m.orbit, m.glyph.Prop("text"))
	}
	m.retint(light)
	if !reflect.DeepEqual(m.orbit.Prop("uniforms"), tint(light)) {
		t.Fatalf("after the theme change the orbit is tinted %v", m.orbit.Prop("uniforms"))
	}
	m.set(light, icCheck, "$stReady")
	if m.orbit != nil || len(m.box.Children()) != 1 || m.glyph.Prop("text") != icCheck {
		t.Fatalf("a ready mark keeps the orbit or loses its glyph")
	}
}
