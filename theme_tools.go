package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var tokenRoles = map[string]string{
	"bg": "window background", "ink": "main text", "inkDim": "secondary text", "inkFaint": "hints and labels",
	"panel": "tiles and bars", "panelAlt": "side list and status bar", "panelInset": "inset fields",
	"edge": "strong borders", "edgeSoft": "dividers", "titlebar": "header bar",
	"control": "buttons and selected rows", "controlEdge": "button borders", "accent": "primary buttons and links",
	"stWork": "working state", "stNeed": "needs-you state", "stReady": "done state", "stFail": "failed state", "stIdle": "idle state",
	"phThink": "activity strip: thinking", "phRead": "activity strip: reading", "phEdit": "activity strip: editing",
	"phRun": "activity strip: running commands", "phTest": "activity strip: testing", "phNone": "activity strip: nothing",
	"workBg": "working tile fill", "hover": "hover tint, usually translucent", "needBg": "needs-you card fill",
	"userBg": "your messages and the open session's row", "codeBg": "code blocks",
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

var themeToolDefs = []map[string]any{
	{"name": "get_theme", "description": "Switchboard's current theme, every theme it has, and each colour token of the current theme with what it paints.",
		"inputSchema": obj(map[string]any{})},
	{"name": "save_theme", "description": "Save a custom theme for Switchboard's own window and switch to it. Tokens you leave out are copied from the base theme. Colours are #rrggbb, or #rrggbbaa for translucent ones.",
		"inputSchema": obj(map[string]any{
			"name":   strProp("The theme's name. It cannot be one of the built-in themes. Saving an existing custom name replaces it."),
			"base":   strProp("The theme to copy missing tokens from. Defaults to the current theme."),
			"colors": map[string]any{"type": "object", "description": "Token name to colour, for example {\"bg\": \"#002b36\"}.", "additionalProperties": map[string]any{"type": "string"}},
		}, "name", "colors")},
}

func (m *mcpServer) themeCall(name string, args map[string]any) (string, error) {
	set := m.hub.Settings()
	switch name {
	case "get_theme":
		cur := themeTokens(set, set.Theme)
		var keys []string
		for k := range tokenRoles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var tokens []map[string]string
		for _, k := range keys {
			tokens = append(tokens, map[string]string{"token": k, "color": cur[k], "paints": tokenRoles[k]})
		}
		b, _ := json.MarshalIndent(map[string]any{"current": set.Theme, "themes": themeNames(set), "tokens": tokens}, "", "  ")
		return string(b), nil
	case "save_theme":
		title, _ := args["name"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			return "", fmt.Errorf("a name is required")
		}
		if slices.Contains(builtinThemes, title) {
			return "", fmt.Errorf("%s is a built-in theme; pick another name", title)
		}
		base, _ := args["base"].(string)
		if base = strings.TrimSpace(base); base == "" {
			base = set.Theme
		}
		if !slices.Contains(themeNames(set), base) {
			return "", fmt.Errorf("no theme called %s", base)
		}
		colors, _ := args["colors"].(map[string]any)
		out := map[string]string{}
		for k, c := range themeTokens(set, base) {
			out[k] = c
		}
		var bad []string
		for k, c := range colors {
			s, _ := c.(string)
			switch {
			case tokenRoles[k] == "":
				bad = append(bad, "unknown token "+k)
			case !hexColor.MatchString(s):
				bad = append(bad, fmt.Sprintf("%s: %q is not #rrggbb or #rrggbbaa", k, s))
			default:
				out[k] = strings.ToLower(s)
			}
		}
		if len(bad) > 0 {
			sort.Strings(bad)
			return "", fmt.Errorf("%s", strings.Join(bad, "; "))
		}
		m.hub.UpdateSettings(func(s *Settings) {
			if s.Themes == nil {
				s.Themes = map[string]map[string]string{}
			}
			s.Themes[title] = out
			s.Theme = title
		})
		return fmt.Sprintf("Saved %s and switched to it. It is in Settings with the other themes.", title), nil
	}
	return "", fmt.Errorf("unknown tool %s", name)
}
