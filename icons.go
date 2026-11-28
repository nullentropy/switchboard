package main

import (
	_ "embed"
	"encoding/base64"
	"strings"

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
	case Waiting:
		return icPause
	case Background:
		return icLayers
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

const orbitFrag = `
vec4 effect(vec2 uv) {
  vec2 p = (uv - 0.5) * u_res;
  float s = 0.5 * min(u_res.x, u_res.y);
  float d = length(p);
  float phi = atan(p.y, p.x);
  float g = (d - 0.64 * s) / 0.9;
  float a = 0.14 * exp(-g * g);
  for (int i = 0; i < 3; i++) {
    float k = float(i);
    float r = (0.56 + 0.08 * k) * s;
    float th = u_time * 3.6 + 2.0944 * k + 0.45 * sin(u_time * 0.8 + 2.0 * k);
    float lag = mod(th - phi, 6.28318);
    float w = max(0.11 * s * (1.0 - lag * 0.22), 0.35);
    float e = (d - r) / w;
    float tail = 0.7 * exp(-lag * 1.8) * exp(-e * e);
    vec2 q = p - r * vec2(cos(th), sin(th));
    float h = dot(q, q) / (s * s);
    a += tail + exp(-h * 55.0) + 0.22 * exp(-h * 14.0);
  }
  a = clamp(a, 0.0, 1.0);
  return vec4(vec3(tintR, tintG, tintB) * a, a);
}`

const stillFrag = `vec4 effect(vec2 uv) { return src(uv); }`

type stateMark struct {
	box, glyph, orbit *caution.Node
	color             string
	tint              [3]float64
}

func newMark(set Settings, glyph string, size float64, color string) *stateMark {
	m := &stateMark{glyph: icon("", size, color).Anchor(caution.A{CenterX: px(0), CenterY: px(0)})}
	m.box = caution.Panel().W(size + 4).H(size + 4).Kids(m.glyph)
	m.set(set, glyph, color)
	return m
}

func (m *stateMark) set(set Settings, glyph, color string) {
	m.color = color
	if glyph != icLoader {
		if m.orbit != nil {
			m.orbit.Remove()
			m.orbit = nil
		}
		m.glyph.SetText(glyph).SetColor(color)
		return
	}
	m.glyph.SetText("")
	if m.orbit == nil {
		m.tint = markTint(set, color)
		m.orbit = caution.Shader(caution.FX{Frag: orbitFrag, Animate: true, Uniforms: tintUniforms(m.tint)}).Anchor(fill())
		m.box.Add(m.orbit)
		return
	}
	m.retint(set)
}

func (m *stateMark) retint(set Settings) {
	if m.orbit == nil {
		return
	}
	if tint := markTint(set, m.color); tint != m.tint {
		m.tint = tint
		m.orbit.SetUniforms(tintUniforms(tint))
	}
}

func markTint(set Settings, color string) [3]float64 {
	r, g, b := tokenRGB(set, strings.TrimPrefix(color, "$"))
	return [3]float64{r, g, b}
}

func tintUniforms(c [3]float64) map[string]float64 {
	return map[string]float64{"tintR": c[0], "tintG": c[1], "tintB": c[2]}
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
