package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
)

const (
	spriteW     = 48
	spriteH     = 10
	spriteScale = 4
)

type canvas [spriteH][spriteW]byte

func (c *canvas) put(x, y int, ch byte) {
	if ch == ' ' || x < 0 || y < 0 || x >= spriteW || y >= spriteH {
		return
	}
	c[y][x] = ch
}

func (c *canvas) draw(x, y int, rows ...string) {
	for dy, row := range rows {
		for dx := 0; dx < len(row); dx++ {
			c.put(x+dx, y+dy, row[dx])
		}
	}
}

var spriteInk = map[byte]string{
	'#': "ink", '+': "inkDim", '.': "inkFaint", 'a': "accent",
	'w': "stWork", 'n': "stNeed", 'r': "stReady", 'f': "stFail",
}

type sprite struct {
	name   string
	frames int
	still  int
	paint  func(c *canvas, i int)
}

type SpriteDef struct {
	Frames [][]string `json:"frames"`
	Still  int        `json:"still,omitempty"`
}

const maxSpriteFrames = 80

func customSprite(name string, d SpriteDef) sprite {
	return sprite{name, len(d.Frames), d.Still, func(c *canvas, i int) {
		c.draw(0, 0, d.Frames[i%len(d.Frames)]...)
	}}
}

func builtinSprite(name string) (sprite, bool) {
	for _, s := range sprites {
		if s.name == name {
			return s, true
		}
	}
	return sprite{}, false
}

func spriteNamed(set Settings, name string) (sprite, bool) {
	if s, ok := builtinSprite(name); ok {
		return s, true
	}
	if d, ok := set.Sprites[name]; ok && len(d.Frames) > 0 {
		return customSprite(name, d), true
	}
	return sprites[0], false
}

func spriteNames(set Settings) []string {
	var names, custom []string
	for _, s := range sprites {
		names = append(names, s.name)
	}
	for n, d := range set.Sprites {
		if _, ok := builtinSprite(n); !ok && len(d.Frames) > 0 {
			custom = append(custom, n)
		}
	}
	sort.Strings(custom)
	return append(names, custom...)
}

func spritesOn(set Settings) []string {
	var on []string
	for _, n := range spriteNames(set) {
		if !slices.Contains(set.SpritesOff, n) {
			on = append(on, n)
		}
	}
	return on
}

func spritePool(set Settings) []string {
	if on := spritesOn(set); len(on) > 0 {
		return on
	}
	var all []string
	for _, s := range sprites {
		all = append(all, s.name)
	}
	return all
}

func spriteText(sp sprite, i int) []string {
	var c canvas
	sp.paint(&c, i)
	rows := make([]string, spriteH)
	last := 0
	for y := range spriteH {
		b := make([]byte, spriteW)
		for x := range spriteW {
			b[x] = ' '
			if c[y][x] != 0 {
				b[x] = c[y][x]
			}
		}
		rows[y] = strings.TrimRight(string(b), " ")
		if rows[y] != "" {
			last = y + 1
		}
	}
	return rows[:last]
}

var sprites = []sprite{
	{"towel", 16, 0, paintTowel},
	{"petunias", 40, 30, paintPetunias},
	{"teacup", 12, 0, paintTeacup},
	{"babel fish", 80, 20, paintFish},
	{"robot", 48, 0, paintRobot},
	{"mice", 8, 0, paintMice},
	{"planet", 40, 32, paintPlanet},
	{"42", 48, 30, paintFortyTwo},
}

func paintTowel(c *canvas, i int) {
	for x := 5; x <= 42; x++ {
		c.put(x, 1, '.')
	}
	for y := 1; y < spriteH; y++ {
		c.put(4, y, '+')
		c.put(43, y, '+')
	}
	for r := range 7 {
		off := int(math.Round(float64(r) / 6 * 3.5 * math.Sin(2*math.Pi*float64(i)/16-float64(r)*0.7)))
		ch := byte('a')
		if r == 4 {
			ch = '#'
		}
		for x := 17; x <= 30; x++ {
			c.put(x+off, 2+r, ch)
		}
	}
	c.put(18, 1, '#')
	c.put(29, 1, '#')
}

var petunias = []string{
	" a a a ",
	"aaaaaaa",
	" rrrrr ",
	"+++++++",
	" +++++ ",
	"  +++  ",
}

func paintPetunias(c *canvas, i int) {
	y := 4
	switch {
	case i < 20:
		t := float64(i) / 19
		y = -6 + int(math.Round(10*t*t))
	case i < 22:
		y = 3
	}
	c.draw(20, y, petunias...)
	if i >= 20 && i < 24 {
		d := i - 20
		c.put(18-d, 9, '.')
		c.put(28+d, 9, '.')
	}
}

var teacup = []string{
	"+++++++++  ",
	"+nnnnnnn+++",
	"+nnnnnnn+ +",
	" +nnnnn+++ ",
	"  +++++    ",
}

func paintTeacup(c *canvas, i int) {
	c.draw(19, 4, teacup...)
	c.draw(17, 9, "+++++++++++++")
	for k, x := range []int{21, 23, 25} {
		p := (i + k*2) % 4
		w := ((i/2 + k) % 2)
		c.put(x+w, 3-p, '.')
	}
}

var fishRight = []string{
	"   nnn   ",
	"n nnnnnn ",
	"nnnnnnn#n",
	"n nnnnnn ",
	"   nnn   ",
}

func mirror(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		b := []byte(r)
		for l, h := 0, len(b)-1; l < h; l, h = l+1, h-1 {
			b[l], b[h] = b[h], b[l]
		}
		out[i] = string(b)
	}
	return out
}

var fishLeft = mirror(fishRight)

func paintFish(c *canvas, i int) {
	y := 2 + (i/4)%2
	if i < 40 {
		c.draw(i, y, fishRight...)
		c.put(i+9, y-1-(i%6)/2, '.')
		return
	}
	x := 39 - (i - 40)
	c.draw(x, y, fishLeft...)
	c.put(x-1, y-1-(i%6)/2, '.')
}

func paintRobot(c *canvas, i int) {
	x, walking := 10, false
	switch {
	case i < 16:
		x, walking = 10+i, true
	case i < 24:
		x = 25
	case i < 40:
		x, walking = 25-(i-24), true
	}
	eyes := " +w+w+ "
	if !walking {
		eyes = " +.+.+ "
		k := i % 8
		c.put(x+7+k/3, 3-k/3, '.')
	}
	legs := " ++ ++ "
	if walking && i%2 == 1 {
		legs = "  +++  "
	}
	c.draw(x, 1,
		"   +   ",
		" +++++ ",
		eyes,
		" +++++ ",
		"  +++  ",
		"+++++++",
		"  +++  ",
		"  + +  ",
		legs,
	)
}

func ring(c *canvas, cx, cy int, r float64, ch byte) {
	for y := range spriteH {
		for x := range spriteW {
			d := math.Hypot(float64(x-cx), float64(y-cy))
			if math.Abs(d-r) < 0.5 {
				c.put(x, y, ch)
			}
		}
	}
}

func paintMice(c *canvas, i int) {
	for w, cx := range []int{16, 31} {
		ring(c, cx, 5, 4, '+')
		th := -float64(i+w) * math.Pi / 8
		for k := range 6 {
			a := th + float64(k)*math.Pi/3
			c.put(cx+int(math.Round(3*math.Cos(a))), 5+int(math.Round(3*math.Sin(a))), '.')
		}
		c.put(cx, 5, '+')
		legs := "+ +"
		if (i+w)%2 == 1 {
			legs = " + "
		}
		c.draw(cx-3, 6, "  ###", ".####")
		c.draw(cx-1, 8, legs)
	}
}

func paintPlanet(c *canvas, i int) {
	const cx, cy = 24, 5
	built := 10
	if i < 30 {
		built = i / 3
		for y := range spriteH {
			c.put(17, y, '.')
			c.put(31, y, '.')
		}
		for x := 18; x < 31; x += 2 {
			c.put(x, 1, '.')
			c.put(x, 9, '.')
		}
	}
	for y := spriteH - built; y < spriteH; y++ {
		for x := cx - 5; x <= cx+5; x++ {
			if math.Hypot(float64(x-cx), float64(y-cy)) > 4.4 {
				continue
			}
			ch := byte('a')
			if (x*7+y*3)%5 < 2 {
				ch = 'r'
			}
			c.put(x, y, ch)
		}
	}
	if i < 30 {
		c.put(20+i%9, spriteH-built-1, '#')
	} else if i%4 < 2 {
		c.put(34, 1, '#')
	}
}

var fortyTwo = []string{
	"#  #  ####",
	"#  #     #",
	"#  #     #",
	"####  ####",
	"   #  #   ",
	"   #  #   ",
	"   #  ####",
}

type dot struct{ sx, sy, tx, ty float64 }

var fortyTwoDots = func() []dot {
	var ds []dot
	seed := uint32(42)
	next := func(n int) float64 {
		seed = seed*1664525 + 1013904223
		return float64((seed >> 8) % uint32(n))
	}
	for y, row := range fortyTwo {
		for x := range len(row) {
			if row[x] == '#' {
				ds = append(ds, dot{next(spriteW), next(spriteH), float64(19 + x), float64(1 + y)})
			}
		}
	}
	return ds
}()

func paintFortyTwo(c *canvas, i int) {
	var t float64
	switch {
	case i < 12:
		t = 0
	case i < 24:
		t = float64(i-12) / 11
	case i < 36:
		t = 1
	default:
		t = 1 - float64(i-36)/11
	}
	t = t * t * (3 - 2*t)
	for k, d := range fortyTwoDots {
		wob := (1 - t) * math.Sin(float64(i)*0.5+float64(k))
		x := d.sx + (d.tx-d.sx)*t + wob
		y := d.sy + (d.ty-d.sy)*t
		c.put(int(math.Round(x)), int(math.Round(y)), 'a')
	}
}

type spriteKey struct {
	c   canvas
	pal string
}

var spriteCache = struct {
	sync.Mutex
	m map[spriteKey]string
}{m: map[spriteKey]string{}}

func spritePalette(set Settings) (string, map[byte]color.NRGBA) {
	cols := map[byte]color.NRGBA{}
	var sig strings.Builder
	for ch, tok := range spriteInk {
		r, g, b := tokenRGB(set, tok)
		col := color.NRGBA{uint8(r*255 + 0.5), uint8(g*255 + 0.5), uint8(b*255 + 0.5), 255}
		cols[ch] = col
	}
	for _, ch := range []byte("#+.awnrf") {
		col := cols[ch]
		sig.Write([]byte{col.R, col.G, col.B})
	}
	return sig.String(), cols
}

func spriteFrame(set Settings, name string, i int) string {
	sig, cols := spritePalette(set)
	sp, _ := spriteNamed(set, name)
	var c canvas
	sp.paint(&c, i)
	key := spriteKey{c, sig}
	spriteCache.Lock()
	defer spriteCache.Unlock()
	if u, ok := spriteCache.m[key]; ok {
		return u
	}
	img := image.NewNRGBA(image.Rect(0, 0, spriteW*spriteScale, spriteH*spriteScale))
	for y := range spriteH {
		for x := range spriteW {
			if c[y][x] == 0 {
				continue
			}
			col := cols[c[y][x]]
			for dy := range spriteScale {
				for dx := range spriteScale {
					img.SetNRGBA(x*spriteScale+dx, y*spriteScale+dy, col)
				}
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	u := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	spriteCache.m[key] = u
	return u
}
