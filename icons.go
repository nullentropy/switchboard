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
vec2 orbitAt(float t, float k) {
  float w = 0.8975979;
  float ph = 2.0943951 * k;
  float th = 3.5903916 * t + 0.55 * sin(4.0 * w * t) + 0.4 * sin(w * t + ph) + ph;
  float v = 3.5903916 + 1.9747154 * cos(4.0 * w * t) + 0.3590392 * cos(w * t + ph);
  return vec2(th, v);
}

vec4 effect(vec2 uv) {
  vec2 p = (uv - 0.5) * u_res;
  float s = 0.5 * min(u_res.x, u_res.y);
  // every rate below is a whole multiple of 2pi/7, so wrapping here is seamless and keeps sin() precise in long sessions
  float t = mod(u_time, 7.0);
  float w = 0.8975979;
  float d = length(p);
  float phi = atan(p.y, p.x);
  float r = s * (0.62 + 0.035 * sin(2.0 * w * t));
  float g = (d - r) / 0.9;
  vec3 tint = vec3(tintR, tintG, tintB);
  vec3 kx = cross(vec3(0.57735), tint) * 0.2474;
  vec3 kd = vec3(dot(vec3(0.33333), tint)) * 0.0311 + tint * 0.9689;
  vec3 warm = clamp(kd + kx, 0.0, 1.0);
  vec3 cool = clamp(kd - kx, 0.0, 1.0);
  float a = 0.05 * exp(-g * g);
  vec3 col = tint * a;
  for (int i = 0; i < 3; i++) {
    float k = float(i);
    vec2 o = orbitAt(t, k);
    float th = o.x;
    float lag = mod(th - phi, 6.28318);
    float tw = max(0.1 * s * exp(-lag * 0.9), 0.4);
    float e = (d - r) / tw;
    float tail = 0.8 * exp(-lag * 4.5 / o.y) * exp(-e * e);
    vec2 q = p - r * vec2(cos(th), sin(th));
    float h = dot(q, q) / (s * s);
    float head = exp(-h * 70.0) + 0.2 * exp(-h * 22.0);
    a += tail + head;
    col += mix(cool, warm, exp(-lag * 0.9)) * tail + warm * head;
    for (int j = 0; j < 2; j++) {
      float c = t / 0.7 + 0.5 * float(j) + 0.33333 * k;
      float age = fract(c);
      float n = mod(floor(c), 10.0);
      float rnd = fract(sin(n * 12.9898 + k * 78.233) * 43758.5453);
      float ts = t - age * 0.7;
      float sa = orbitAt(ts, k).x + (rnd - 0.5) * 0.6;
      float sr = s * (0.62 + 0.035 * sin(2.0 * w * ts) + age * (0.2 + 0.16 * rnd));
      vec2 sq = p - sr * vec2(cos(sa), sin(sa));
      float spark = 0.75 * (1.0 - age) * (1.0 - age) * exp(-dot(sq, sq) / (s * s) * 110.0);
      a += spark;
      col += warm * spark;
    }
    // comets k and k+1 are closest when this cosine is -1
    float cm = cos(w * t + 2.0943951 * k + 1.0471976);
    float mid = th + 1.0471976 + 0.3464102 * cm;
    vec2 m = p - r * vec2(cos(mid), sin(mid));
    vec2 am = abs(m) / s;
    float star = exp(-dot(am, am) * 220.0) + 0.45 * (exp(-am.x * 7.0 - am.y * am.y * 600.0) + exp(-am.y * 7.0 - am.x * am.x * 600.0));
    float glint = 0.6 * exp(-12.0 * (1.0 + cm)) * star;
    a += glint;
    col += mix(warm, vec3(1.0), 0.3) * glint;
  }
  float alpha = clamp(a, 0.0, 1.0);
  col = min(col * (alpha / max(a, 0.0001)), vec3(alpha));
  return vec4(col, alpha);
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
