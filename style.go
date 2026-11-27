package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"math"
	"slices"
	"strings"
)

var themes = map[string]map[string]string{
	"Graphite": {
		"bg": "#111317", "ink": "#e6e8ec", "inkDim": "#a0a6b1", "inkFaint": "#6c7380",
		"panel": "#1a1d23", "panelAlt": "#16191e", "panelInset": "#0e1013",
		"edge": "#2b3039", "edgeSoft": "#242830", "titlebar": "#16191e",
		"control": "#232730", "controlEdge": "#363c48", "accent": "#5b8cff",
		"stWork": "#5b8cff", "stNeed": "#f5a524", "stReady": "#3ecf8e", "stFail": "#f0525c", "stIdle": "#5d6470", "stBg": "#a78bfa",
		"phThink": "#4a5160", "phRead": "#3fa7c9", "phEdit": "#8b6cff", "phRun": "#5b8cff", "phTest": "#3ecf8e", "phNone": "#22262e",
		"workBg": "#1f2532", "hover": "#ffffff0a", "needBg": "#2a2114", "userBg": "#1f2633", "codeBg": "#0e1013",
	},
	"Paper": {
		"bg": "#eef0f3", "ink": "#1a1d23", "inkDim": "#545c69", "inkFaint": "#8a919c",
		"panel": "#ffffff", "panelAlt": "#f7f8fa", "panelInset": "#f0f2f5",
		"edge": "#d6dae1", "edgeSoft": "#e4e7ec", "titlebar": "#f7f8fa",
		"control": "#f2f3f6", "controlEdge": "#c9ced7", "accent": "#2f6bff",
		"stWork": "#2f6bff", "stNeed": "#d98a00", "stReady": "#169c63", "stFail": "#d93644", "stIdle": "#9aa1ab", "stBg": "#7c4dff",
		"phThink": "#c3c8d0", "phRead": "#1f8fb5", "phEdit": "#7351f0", "phRun": "#2f6bff", "phTest": "#169c63", "phNone": "#e6e9ee",
		"workBg": "#f2f6ff", "hover": "#0000000a", "needBg": "#fff6e5", "userBg": "#eaf0ff", "codeBg": "#f0f2f5",
	},
	"Phosphor": {
		"bg": "#050a06", "ink": "#9dffb0", "inkDim": "#5fc77a", "inkFaint": "#3a7a4b",
		"panel": "#0a140c", "panelAlt": "#08100a", "panelInset": "#040805",
		"edge": "#1d3a24", "edgeSoft": "#142a19", "titlebar": "#08100a",
		"control": "#0f2214", "controlEdge": "#2a5a35", "accent": "#23a24a",
		"stWork": "#5dff86", "stNeed": "#ffd23f", "stReady": "#8dffa8", "stFail": "#ff5f56", "stIdle": "#2f5f3b", "stBg": "#4dd2c4",
		"phThink": "#1f4a2a", "phRead": "#3fd0a0", "phEdit": "#b0ff5d", "phRun": "#5dff86", "phTest": "#8dffa8", "phNone": "#0d1a10",
		"workBg": "#102415", "hover": "#9dffb010", "needBg": "#1c1a07", "userBg": "#0d2112", "codeBg": "#040805",
	},
	"Amber": {
		"bg": "#0b0702", "ink": "#ffcf7a", "inkDim": "#c9953f", "inkFaint": "#7d5a22",
		"panel": "#150e04", "panelAlt": "#110b03", "panelInset": "#080501",
		"edge": "#3d2a0d", "edgeSoft": "#2c1e09", "titlebar": "#110b03",
		"control": "#22170a", "controlEdge": "#5a3e14", "accent": "#ffb347",
		"stWork": "#ffb347", "stNeed": "#ff6b3d", "stReady": "#ffe0a3", "stFail": "#ff4d3d", "stIdle": "#6a4c1c", "stBg": "#e8a0ff",
		"phThink": "#4a3410", "phRead": "#ffd27a", "phEdit": "#ff9f43", "phRun": "#ffb347", "phTest": "#ffe0a3", "phNone": "#1c1306",
		"workBg": "#251a09", "hover": "#ffcf7a10", "needBg": "#2a1206", "userBg": "#221607", "codeBg": "#080501",
	},
	"Gruvbox": {
		"bg": "#1d2021", "ink": "#ebdbb2", "inkDim": "#bdae93", "inkFaint": "#928374",
		"panel": "#282828", "panelAlt": "#232425", "panelInset": "#171919",
		"edge": "#504945", "edgeSoft": "#3c3836", "titlebar": "#232425",
		"control": "#3c3836", "controlEdge": "#665c54", "accent": "#458588",
		"stWork": "#83a598", "stNeed": "#fabd2f", "stReady": "#b8bb26", "stFail": "#fb4934", "stIdle": "#7c6f64", "stBg": "#d3869b",
		"phThink": "#504945", "phRead": "#8ec07c", "phEdit": "#d3869b", "phRun": "#83a598", "phTest": "#b8bb26", "phNone": "#32302f",
		"workBg": "#26302f", "hover": "#ebdbb20a", "needBg": "#352a17", "userBg": "#32302f", "codeBg": "#171919",
	},
}

var builtinThemes = []string{"Graphite", "Paper", "Phosphor", "Amber", "Gruvbox"}

func themeNames(set Settings) []string {
	names := append([]string(nil), builtinThemes...)
	var custom []string
	for n := range set.Themes {
		if themes[n] == nil {
			custom = append(custom, n)
		}
	}
	slices.Sort(custom)
	return append(names, custom...)
}

func themeTokens(set Settings, name string) map[string]string {
	if t := themes[name]; t != nil {
		return t
	}
	if t := set.Themes[name]; t != nil {
		if t["stBg"] != "" {
			return t
		}
		out := map[string]string{"stBg": themes["Gruvbox"]["stBg"]}
		for k, v := range t {
			out[k] = v
		}
		return out
	}
	return themes["Gruvbox"]
}

func stateToken(s State) string {
	switch s {
	case Working, Starting:
		return "stWork"
	case NeedsYou:
		return "stNeed"
	case Ready:
		return "stReady"
	case Failed:
		return "stFail"
	case Interrupted, Waiting:
		return "stNeed"
	case Background:
		return "stBg"
	}
	return "stIdle"
}

func tokenRGB(set Settings, tok string) (r, g, b float64) {
	h := strings.ToLower(themeTokens(set, set.Theme)[tok])
	if len(h) < 7 {
		return 1, 1, 1
	}
	var v [3]float64
	for i := range 3 {
		var x int
		for _, c := range h[1+2*i : 3+2*i] {
			x *= 16
			switch {
			case c >= '0' && c <= '9':
				x += int(c - '0')
			case c >= 'a' && c <= 'f':
				x += int(c-'a') + 10
			}
		}
		v[i] = float64(x) / 255
	}
	return v[0], v[1], v[2]
}

const barFrag = `
vec4 effect(vec2 uv) {
  float x = fract(u_time * 0.6);
  float d = uv.x - x;
  float h = exp(-d * d * 60.0);
  vec3 tint = vec3(tintR, tintG, tintB);
  float a = 0.55 + 0.45 * h;
  return vec4(tint * a, a);
}`

var soundNeed = synth([]tone{{392, 0.32}, {523.25, 0.55}}, 0.10)

type tone struct {
	hz, sec float64
}

func synth(tones []tone, vol float64) string {
	const rate = 44100
	var pcm []int16
	for _, t := range tones {
		n := int(t.sec * rate)
		for i := range n {
			env := math.Min(1, float64(i)/(rate*0.02)) * math.Exp(-4*float64(i)/float64(n))
			v := 0.0
			if t.hz > 0 {
				ph := 2 * math.Pi * t.hz * float64(i) / rate
				v = (math.Sin(ph) + 0.08*math.Sin(2*ph)) * env * vol
			}
			pcm = append(pcm, int16(v*32767))
		}
	}
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(pcm)*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(pcm) * 2))
	w(pcm)
	return "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
