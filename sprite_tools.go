package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var spriteToolDefs = []map[string]any{
	{"name": "get_sprite", "description": "Switchboard's busy animations, shown beside the activity line while a session works. Without a name: the grid size, the palette characters and the colour token each paints, the frame rules, and every sprite with whether it is built in and on. With a name: that sprite's frames as text.",
		"inputSchema": obj(map[string]any{"name": strProp("A sprite's name, to get its frames.")})},
	{"name": "save_sprite", "description": fmt.Sprintf("Save a custom busy animation and turn it on. Each frame is up to %d rows of up to %d characters drawn from the palette; a space is transparent. %d frames at most, shown every 125ms.", spriteH, spriteW, maxSpriteFrames),
		"inputSchema": obj(map[string]any{
			"name":   strProp("The sprite's name. It cannot be a built-in sprite's name. Saving an existing custom name replaces it."),
			"frames": map[string]any{"type": "array", "description": "The frames in order. Each frame is a list of rows, top to bottom.", "items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			"still":  map[string]any{"type": "integer", "description": "The frame shown when the system asks for reduced motion, counting from 0. Defaults to 0."},
		}, "name", "frames")},
	{"name": "delete_sprite", "description": "Delete a custom busy animation. Built-in sprites cannot be deleted; the user turns them off in Settings.",
		"inputSchema": obj(map[string]any{"name": strProp("The custom sprite's name.")}, "name")},
}

func (m *mcpServer) spriteCall(name string, args map[string]any) (string, error) {
	set := m.hub.Settings()
	title, _ := args["name"].(string)
	title = strings.TrimSpace(title)
	switch name {
	case "get_sprite":
		if title != "" {
			sp, ok := spriteNamed(set, title)
			if !ok {
				return "", fmt.Errorf("no sprite called %s", title)
			}
			frames := make([][]string, sp.frames)
			for i := range frames {
				frames[i] = spriteText(sp, i)
			}
			b, _ := json.MarshalIndent(map[string]any{"name": title, "still": sp.still, "frames": frames}, "", "  ")
			return string(b), nil
		}
		var palette []map[string]string
		for _, ch := range []byte("#+.awnrf") {
			tok := spriteInk[ch]
			palette = append(palette, map[string]string{"char": string(ch), "token": tok, "paints": tokenRoles[tok]})
		}
		on := spritesOn(set)
		var list []map[string]any
		for _, n := range spriteNames(set) {
			_, builtin := builtinSprite(n)
			list = append(list, map[string]any{"name": n, "builtin": builtin, "on": slices.Contains(on, n)})
		}
		b, _ := json.MarshalIndent(map[string]any{
			"width": spriteW, "height": spriteH, "maxFrames": maxSpriteFrames, "frameMs": busyFrame.Milliseconds(),
			"transparent": " ", "palette": palette, "sprites": list,
		}, "", "  ")
		return string(b), nil
	case "save_sprite":
		if title == "" {
			return "", fmt.Errorf("a name is required")
		}
		if _, ok := builtinSprite(title); ok {
			return "", fmt.Errorf("%s is a built-in sprite; pick another name", title)
		}
		raw, _ := args["frames"].([]any)
		var bad []string
		if len(raw) == 0 || len(raw) > maxSpriteFrames {
			bad = append(bad, fmt.Sprintf("%d frames; a sprite has 1 to %d", len(raw), maxSpriteFrames))
		}
		frames := make([][]string, len(raw))
		for f, fr := range raw {
			rows, ok := fr.([]any)
			if !ok {
				bad = append(bad, fmt.Sprintf("frame %d is not a list of rows", f))
				continue
			}
			if len(rows) > spriteH {
				bad = append(bad, fmt.Sprintf("frame %d has %d rows; at most %d", f, len(rows), spriteH))
			}
			for r, row := range rows {
				s, ok := row.(string)
				switch {
				case !ok:
					bad = append(bad, fmt.Sprintf("frame %d row %d is not text", f, r))
					continue
				case len(s) > spriteW:
					bad = append(bad, fmt.Sprintf("frame %d row %d is %d wide; at most %d", f, r, len(s), spriteW))
				}
				var unknown []string
				for _, ch := range s {
					if c := string(ch); ch != ' ' && (ch > 0x7f || spriteInk[byte(ch)] == "") && !slices.Contains(unknown, c) {
						unknown = append(unknown, c)
					}
				}
				if len(unknown) > 0 {
					bad = append(bad, fmt.Sprintf("frame %d row %d uses %q, not in the palette", f, r, strings.Join(unknown, "")))
				}
				frames[f] = append(frames[f], s)
			}
		}
		still := 0
		if v, ok := args["still"].(float64); ok {
			still = int(v)
			if float64(still) != v || still < 0 || still >= max(len(raw), 1) {
				bad = append(bad, fmt.Sprintf("still frame %v is not one of the frames", v))
			}
		}
		if len(bad) > 0 {
			return "", fmt.Errorf("%s", strings.Join(bad, "; "))
		}
		m.hub.UpdateSettings(func(s *Settings) {
			s.Sprites = maps.Clone(s.Sprites)
			if s.Sprites == nil {
				s.Sprites = map[string]SpriteDef{}
			}
			s.Sprites[title] = SpriteDef{Frames: frames, Still: still}
			s.SpritesOff = withoutName(s.SpritesOff, title)
		})
		return fmt.Sprintf("Saved %s and turned it on. Working sessions pick it at random with the other sprites that are on.", title), nil
	case "delete_sprite":
		if _, ok := builtinSprite(title); ok {
			return "", fmt.Errorf("%s is a built-in sprite; the user can turn it off in Settings", title)
		}
		if _, ok := set.Sprites[title]; !ok {
			return "", fmt.Errorf("no custom sprite called %s", title)
		}
		m.hub.UpdateSettings(func(s *Settings) {
			s.Sprites = maps.Clone(s.Sprites)
			delete(s.Sprites, title)
			s.SpritesOff = withoutName(s.SpritesOff, title)
		})
		return fmt.Sprintf("Deleted %s.", title), nil
	}
	return "", fmt.Errorf("unknown tool %s", name)
}

func withoutName(names []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == name })
}

func setSpriteOn(s *Settings, name string, on bool) bool {
	if !on && slices.Equal(spritesOn(*s), []string{name}) {
		return false
	}
	s.SpritesOff = withoutName(s.SpritesOff, name)
	if !on {
		s.SpritesOff = append(s.SpritesOff, name)
	}
	return true
}
