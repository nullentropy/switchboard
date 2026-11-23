package main

import (
	_ "embed"
	"encoding/base64"

	caution "github.com/nullentropy/caution"
)

//go:embed assets/fonts/Inter.ttf
var interFont []byte

//go:embed assets/fonts/JetBrainsMono.ttf
var monoFont []byte

//go:embed assets/fonts/Lucide.ttf
var iconFont []byte

//go:embed icon.png
var appIcon []byte

func fontURI(b []byte) string { return "data:font/ttf;base64," + base64.StdEncoding.EncodeToString(b) }

var (
	interURI = fontURI(interFont)
	monoURI  = fontURI(monoFont)
	iconURI  = fontURI(iconFont)
)

const (
	icLoader   = ""
	icHand     = ""
	icCheck    = ""
	icFail     = ""
	icPause    = ""
	icPlus     = ""
	icSearch   = ""
	icSparkles = ""
	icTerminal = ""
	icSettings = ""
	icFolder   = ""
	icBranch   = ""
	icLayers   = ""
	icTarget   = ""
	icAlert    = ""
	icSend     = ""
	icBack     = ""
	icBot      = ""
	icZap      = ""
	icKeyboard = ""
	icCircle   = ""
	icSquare   = "\ue167"
	icPower    = "\ue140"
	icRetry    = "\ue148"
	icKeys     = "\ue284"
)

func stateIcon(s State) string {
	switch s {
	case Working, Starting:
		return icLoader
	case NeedsYou:
		return icHand
	case Ready:
		return icCheck
	case Failed:
		return icFail
	case Interrupted:
		return icRetry
	}
	return icPause
}

func agentIcon(provider string) string {
	switch provider {
	case "codex":
		return icTerminal
	case "gemini":
		return icZap
	}
	return icSparkles
}

const spinFrag = `
vec4 effect(vec2 uv) {
  float a = -u_time * 4.0;
  vec2 p = (uv - 0.5) * u_res;
  p = vec2(p.x * cos(a) - p.y * sin(a), p.x * sin(a) + p.y * cos(a));
  vec2 q = p / u_res + 0.5;
  if (q.x < 0.0 || q.y < 0.0 || q.x > 1.0 || q.y > 1.0) return vec4(0.0);
  return src(q);
}`

const stillFrag = `vec4 effect(vec2 uv) { return src(uv); }`

type stateMark struct {
	box, glyph *caution.Node
	spinning   bool
}

func newMark(glyph string, size float64, color string) *stateMark {
	m := &stateMark{glyph: icon("", size, color).Anchor(caution.A{CenterX: px(0), CenterY: px(0)})}
	m.box = caution.Panel().W(size + 4).H(size + 4).Kids(m.glyph)
	m.set(glyph, color)
	return m
}

func (m *stateMark) set(glyph, color string) {
	m.glyph.SetText(glyph).SetColor(color)
	spin := glyph == icLoader
	if spin == m.spinning {
		return
	}
	m.spinning = spin
	if spin {
		m.box.Effect(caution.FX{Frag: spinFrag, Animate: true})
	} else {
		m.box.Effect(caution.FX{Frag: stillFrag})
	}
}

const shimmerFrag = `
vec4 effect(vec2 uv) {
  vec4 c = src(uv);
  float x = mix(-60.0, u_w + 60.0, fract(u_time / 3.2));
  float band = exp(-pow((uv.x * u_res.x - x) / 28.0, 2.0));
  c.rgb = mix(c.rgb, vec3(u_r, u_g, u_b) * c.a, band * 0.55);
  return c;
}`

type glint struct {
	n    *caution.Node
	on   bool
	w    float64
	tint [3]float64
}

func (g *glint) set(set Settings, text string, size float64, mono, on bool) {
	if on {
		w := textW(text, size, mono)
		var tint [3]float64
		tint[0], tint[1], tint[2] = tokenRGB(set, "accent")
		if !g.on || w != g.w || tint != g.tint {
			g.w, g.tint = w, tint
			g.n.Effect(caution.FX{Frag: shimmerFrag, Animate: true,
				Uniforms: map[string]float64{"u_w": w, "u_r": tint[0], "u_g": tint[1], "u_b": tint[2]}})
		}
	} else if g.on {
		g.n.Effect(caution.FX{Frag: stillFrag})
	}
	g.on = on
}

func (g *glint) retint(set Settings) {
	if g.on {
		var tint [3]float64
		tint[0], tint[1], tint[2] = tokenRGB(set, "accent")
		if tint != g.tint {
			g.tint = tint
			g.n.Effect(caution.FX{Frag: shimmerFrag, Animate: true,
				Uniforms: map[string]float64{"u_w": g.w, "u_r": tint[0], "u_g": tint[1], "u_b": tint[2]}})
		}
	}
}

func thinking(s Snapshot) bool { return s.State == Working && s.Activity == "Thinking" }
