package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var uiSlotRoles = map[string]string{
	"header":  "the title bar row",
	"board":   "the wall under the header",
	"tile":    "one session's card on the board; s is the session",
	"railRow": "one session's row in the side rail; s is the session",
	"empty":   "what shows with no sessions",
}

var uiComponentRoles = map[string]string{
	"sb.tile":     "the default session tile",
	"sb.sprite":   "the busy animation",
	"sb.activity": "the activity strip",
	"sb.stateDot": "the session's state icon",
	"sb.inbox":    "the Needs-you cards; a board without one gets the Go inbox at the top",
	"sb.limits":   "the usage limits bar",
}

var uiPropKindDocs = map[uiPropKind]string{
	uiPNum:      "number",
	uiPBool:     "true or false",
	uiPColour:   "colour: a theme token such as $accent, or #rrggbb",
	uiPText:     "text, may hold {{path}}",
	uiPAlign:    "start, center, end or stretch",
	uiPAnchor:   "fill, center, top, bottom, left or right",
	uiPColumns:  "number of columns",
	uiPFraction: "number from 0 to 1",
	uiPDataURI:  "image data URI",
}

var uiToolDefs = []map[string]any{
	{"name": "get_ui", "description": "Switchboard's scripted interfaces. Without a name: the format, the slots, the data fields, the actions, the built-in components, every saved interface and which one is on. With a name: that interface's JSON.",
		"inputSchema": obj(map[string]any{"name": strProp("A saved interface's name, to get its JSON.")})},
	{"name": "save_ui", "description": fmt.Sprintf("Save an interface for Switchboard's window and switch to it. It is checked, then every slot is rendered against the current data off screen; any error refuses the save and is returned. At most %d interfaces and %d KB each. Saving an existing name replaces it. %s is reserved.", uiMaxInterfaces, uiMaxBytes>>10, uiDefaultName),
		"inputSchema": obj(map[string]any{
			"interface": map[string]any{"type": "object", "description": "The interface: {\"name\": ..., \"slots\": {...}} in the format get_ui describes."},
		}, "interface")},
	{"name": "use_ui", "description": fmt.Sprintf("Switch Switchboard's window to a saved interface, or to %s.", uiDefaultName),
		"inputSchema": obj(map[string]any{"name": strProp(fmt.Sprintf("A saved interface's name, or %s.", uiDefaultName))}, "name")},
	{"name": "delete_ui", "description": fmt.Sprintf("Delete a saved interface. Deleting the one that is on switches to %s.", uiDefaultName),
		"inputSchema": obj(map[string]any{"name": strProp("The saved interface's name.")}, "name")},
}

func (m *mcpServer) uiCall(name string, args map[string]any) (string, error) {
	set := m.hub.Settings()
	title, _ := args["name"].(string)
	title = strings.TrimSpace(title)
	switch name {
	case "get_ui":
		if title != "" {
			if strings.EqualFold(title, uiDefaultName) {
				return "", fmt.Errorf("%s is built into Switchboard and has no JSON", uiDefaultName)
			}
			raw, ok := set.Interfaces[title]
			if !ok {
				return "", fmt.Errorf("no interface called %s", title)
			}
			return string(raw), nil
		}
		b, _ := json.MarshalIndent(m.uiGuide(set), "", "  ")
		return string(b), nil
	case "save_ui":
		raw, err := uiArgJSON(args["interface"])
		if err != nil {
			return "", err
		}
		ui, errs := parseUI(raw)
		if len(errs) > 0 {
			return "", fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		title = strings.TrimSpace(ui.Name)
		saved := map[string]string{}
		for k := range set.Interfaces {
			saved[k] = ""
		}
		if err := checkUIName(title, saved); err != nil {
			return "", err
		}
		if errs := m.uiDryRender(ui); len(errs) > 0 {
			return "", fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		m.hub.UseUI(func(s *Settings) {
			s.Interfaces = maps.Clone(s.Interfaces)
			if s.Interfaces == nil {
				s.Interfaces = map[string]json.RawMessage{}
			}
			s.Interfaces[title] = raw
			s.Interface = title
		})
		return fmt.Sprintf("Saved %s and switched to it. A slot that fails to render shows the Go default with a notice.", title), nil
	case "use_ui":
		if title == "" {
			return "", fmt.Errorf("a name is required")
		}
		if strings.EqualFold(title, uiDefaultName) {
			m.hub.UseUI(func(s *Settings) { s.Interface = "" })
			return fmt.Sprintf("Switched to %s.", uiDefaultName), nil
		}
		if _, ok := set.Interfaces[title]; !ok {
			return "", fmt.Errorf("no interface called %s", title)
		}
		m.hub.UseUI(func(s *Settings) { s.Interface = title })
		return fmt.Sprintf("Switched to %s.", title), nil
	case "delete_ui":
		if strings.EqualFold(title, uiDefaultName) {
			return "", fmt.Errorf("%s is built in and cannot be deleted", uiDefaultName)
		}
		if _, ok := set.Interfaces[title]; !ok {
			return "", fmt.Errorf("no interface called %s", title)
		}
		wasOn := false
		m.hub.UpdateSettings(func(s *Settings) {
			s.Interfaces = maps.Clone(s.Interfaces)
			delete(s.Interfaces, title)
			if s.Interface == title {
				s.Interface = ""
				wasOn = true
			}
		})
		if wasOn {
			return fmt.Sprintf("Deleted %s and switched to %s.", title, uiDefaultName), nil
		}
		return fmt.Sprintf("Deleted %s.", title), nil
	}
	return "", fmt.Errorf("unknown tool %s", name)
}

func uiArgJSON(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("an interface is required")
	case string:
		var probe map[string]any
		if err := json.Unmarshal([]byte(x), &probe); err != nil {
			return nil, fmt.Errorf("invalid interface JSON: %s", strings.TrimPrefix(err.Error(), "json: "))
		}
		v = probe
	case map[string]any:
	default:
		return nil, fmt.Errorf("the interface must be a JSON object")
	}
	return json.Marshal(v)
}

type uiDryActor struct{}

func (uiDryActor) uiOpenSession(*Agent)        {}
func (uiDryActor) uiNewSession()               {}
func (uiDryActor) uiFocusInput()               {}
func (uiDryActor) uiOpenPalette()              {}
func (uiDryActor) uiNextWaiting()              {}
func (uiDryActor) uiShowBoard()                {}
func (uiDryActor) uiOpenSettings()             {}
func (uiDryActor) uiAnswer(*Agent, bool, bool) {}
func (uiDryActor) uiInterrupt(*Agent)          {}
func (uiDryActor) uiFilterGoal(string)         {}
func (uiDryActor) uiClearFilter()              {}
func (uiDryActor) uiStopSession(*Agent)        {}
func (uiDryActor) uiRemoveSession(*Agent)      {}
func (uiDryActor) uiMarkGoalDone(string)       {}

func (m *mcpServer) uiDryRender(ui *uiInterface) []string {
	v := &View{hub: m.hub, set: m.hub.Settings(), tiles: map[*Agent]*tile{}}
	data := uiData(m.hub, uiViewState{Columns: 3, Width: 1400})
	rows, _ := uiItems(data["sessions"])
	var errs []string
	for _, slot := range slices.Sorted(maps.Keys(ui.Slots)) {
		root := ui.Slots[slot]
		if !uiSessionSlots[slot] {
			if _, err := renderUI(v, uiDryActor{}, root, data, nil); err != nil {
				errs = append(errs, fmt.Sprintf("slots.%s: %v", slot, err))
			}
			continue
		}
		for _, row := range rows {
			if _, err := renderUI(v, uiDryActor{}, root, data, row); err != nil {
				title, _ := row.(map[string]any)["title"].(string)
				errs = append(errs, fmt.Sprintf("slots.%s for %s: %v", slot, title, err))
				break
			}
		}
	}
	return errs
}

func (m *mcpServer) uiGuide(set Settings) map[string]any {
	active, _ := m.hub.ActiveUI()
	on := set.Interface
	if on == "" {
		on = uiDefaultName
	}
	if active == "" {
		active = uiDefaultName
	}
	var saved []map[string]any
	for _, n := range slices.Sorted(maps.Keys(set.Interfaces)) {
		var ui uiInterface
		_ = json.Unmarshal(set.Interfaces[n], &ui)
		saved = append(saved, map[string]any{"name": n, "on": n == set.Interface, "bytes": len(set.Interfaces[n]), "slots": slices.Sorted(maps.Keys(ui.Slots))})
	}
	var slots []map[string]string
	for _, s := range []string{"header", "board", "tile", "railRow", "empty"} {
		slots = append(slots, map[string]string{"slot": s, "replaces": uiSlotRoles[s]})
	}
	props := map[string]string{}
	for k, kind := range uiCommonProps {
		props[k] = uiPropKindDocs[kind]
	}
	typeProps := map[string]map[string]string{}
	for t, ps := range uiTypeProps {
		typeProps[t] = map[string]string{}
		for k, kind := range ps {
			typeProps[t][k] = uiPropKindDocs[kind]
		}
	}
	var actions []map[string]string
	for _, a := range slices.Sorted(maps.Keys(uiActions)) {
		arg := ""
		switch uiActions[a] {
		case uiArgSession:
			arg = "session"
		case uiArgGoal:
			arg = "goal"
		}
		actions = append(actions, map[string]string{"action": a, "takes": arg})
	}
	var comps []map[string]any
	for _, c := range slices.Sorted(maps.Keys(uiComponents)) {
		props := slices.Sorted(maps.Keys(uiSizeProps))
		if uiComponentNeedsSession[c] {
			props = append(props, "session")
		}
		comps = append(comps, map[string]any{"type": c, "draws": uiComponentRoles[c], "props": props})
	}
	return map[string]any{
		"on":      on,
		"showing": active,
		"saved":   saved,
		"format": map[string]any{
			"interface":  `{"name": "...", "slots": {"<slot>": <node>}}`,
			"node":       `{"type", "props", "kids", "text", "each", "as", "if", "onClick"}`,
			"types":      slices.Sorted(maps.Keys(uiNativeTypes)),
			"containers": slices.Sorted(maps.Keys(uiContainers)),
			"textTypes":  slices.Sorted(maps.Keys(uiTextTypes)),
			"props":      props,
			"typeProps":  typeProps,
			"bindings":   "{{path}} puts a value in a string. each names a list path and repeats the node once per item, bound to the name in as. if keeps the node when a condition holds: a path, !path, or a path compared to a number or quoted string with ==, !=, < or >, joined with && or ||.",
			"onClick":    `{"action": "...", "session": "{{s.id}}"} or {"action": "...", "goal": "..."}`,
			"limits":     fmt.Sprintf("%d KB per interface, %d interfaces, %d nodes per rendered slot", uiMaxBytes>>10, uiMaxInterfaces, uiMaxNodes),
		},
		"slots":      slots,
		"data":       uiFieldDoc(uiRootField),
		"actions":    actions,
		"components": comps,
	}
}

func uiFieldDoc(f *uiField) any {
	switch f.kind {
	case uiStr:
		return "text"
	case uiNum:
		return "number"
	case uiMoney:
		return "dollars"
	case uiBool:
		return "bool"
	case uiList:
		return []any{uiFieldDoc(f.item)}
	}
	out := map[string]any{}
	for k, sub := range f.fields {
		out[k] = uiFieldDoc(sub)
	}
	return out
}
