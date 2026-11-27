package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	uiMaxBytes      = 64 << 10
	uiMaxInterfaces = 20
	uiDefaultName   = "Default"
)

var uiSlots = map[string]bool{"header": true, "board": true, "tile": true, "railRow": true, "empty": true}

var uiSessionSlots = map[string]bool{"tile": true, "railRow": true}

type uiInterface struct {
	Name  string             `json:"name"`
	Slots map[string]*uiNode `json:"slots"`
}

type uiNode struct {
	Type    string         `json:"type"`
	Props   map[string]any `json:"props,omitempty"`
	Kids    []*uiNode      `json:"kids,omitempty"`
	Text    string         `json:"text,omitempty"`
	Each    string         `json:"each,omitempty"`
	As      string         `json:"as,omitempty"`
	If      string         `json:"if,omitempty"`
	OnClick *uiAction      `json:"onClick,omitempty"`
}

type uiAction struct {
	Action  string `json:"action"`
	Session string `json:"session,omitempty"`
	Goal    string `json:"goal,omitempty"`
}

type uiKind int

const (
	uiStr uiKind = iota
	uiNum
	uiMoney
	uiBool
	uiList
	uiObj
)

type uiField struct {
	kind   uiKind
	item   *uiField
	fields map[string]*uiField
}

func uiObject(fields map[string]*uiField) *uiField { return &uiField{kind: uiObj, fields: fields} }

var (
	uiFStr   = &uiField{kind: uiStr}
	uiFNum   = &uiField{kind: uiNum}
	uiFMoney = &uiField{kind: uiMoney}
	uiFBool  = &uiField{kind: uiBool}

	uiSessionField = uiObject(map[string]*uiField{
		"id": uiFStr, "title": uiFStr, "agent": uiFStr, "goal": uiFStr, "goalDone": uiFBool,
		"folder": uiFStr, "branch": uiFStr, "state": uiFStr, "activity": uiFStr, "cost": uiFMoney,
		"waiting": uiFStr, "queued": uiFNum, "lastReply": uiFStr, "elapsed": uiFNum,
		"selected": uiFBool, "key": uiFNum,
	})
	uiSessionList = &uiField{kind: uiList, item: uiSessionField}

	uiRootField = uiObject(map[string]*uiField{
		"sessions": uiSessionList,
		"groups": uiObject(map[string]*uiField{
			"need": uiSessionList, "work": uiSessionList, "bg": uiSessionList, "wait": uiSessionList,
			"fail": uiSessionList, "int": uiSessionList, "done": uiSessionList, "idle": uiSessionList,
		}),
		"goals": {kind: uiList, item: uiObject(map[string]*uiField{
			"name": uiFStr, "done": uiFBool, "sessions": uiSessionList,
		})},
		"totals": uiObject(map[string]*uiField{"need": uiFNum, "work": uiFNum, "done": uiFNum, "spent": uiFMoney}),
		"load":   uiObject(map[string]*uiField{"overloaded": uiFBool, "load": uiFNum, "cores": uiFNum, "freeMemory": uiFNum}),
		"limits": {kind: uiList, item: uiObject(map[string]*uiField{
			"provider": uiFStr, "name": uiFStr, "five": uiFNum, "week": uiFNum,
			"hasFive": uiFBool, "hasWeek": uiFBool, "fiveResets": uiFStr, "weekResets": uiFStr,
		})},
		"update": uiObject(map[string]*uiField{"ready": uiFBool, "version": uiFStr}),
		"view":   uiObject(map[string]*uiField{"columns": uiFNum, "width": uiFNum, "goal": uiFStr, "project": uiFStr}),
	})
)

var uiContainers = map[string]bool{"panel": true, "vstack": true, "hstack": true, "grid": true, "scroll": true, "glass": true}

var uiTextTypes = map[string]bool{"label": true, "richtext": true, "button": true}

var uiNativeTypes = map[string]bool{
	"panel": true, "vstack": true, "hstack": true, "grid": true, "scroll": true, "label": true,
	"richtext": true, "progress": true, "button": true, "glass": true, "image": true,
}

var uiComponents = map[string]bool{
	"sb.tile": true, "sb.sprite": true, "sb.activity": true, "sb.stateDot": true, "sb.inbox": true, "sb.limits": true,
}

var uiComponentNeedsSession = map[string]bool{"sb.tile": true, "sb.sprite": true, "sb.activity": true, "sb.stateDot": true}

type uiPropKind int

const (
	uiPNum uiPropKind = iota
	uiPBool
	uiPColour
	uiPText
	uiPAlign
	uiPAnchor
	uiPColumns
	uiPFraction
	uiPDataURI
)

var uiCommonProps = map[string]uiPropKind{
	"width": uiPNum, "height": uiPNum, "fill": uiPNum, "pad": uiPNum, "gap": uiPNum,
	"align": uiPAlign, "anchor": uiPAnchor, "bg": uiPColour, "color": uiPColour, "border": uiPColour,
	"borderWidth": uiPNum, "radius": uiPNum, "fontSize": uiPNum, "weight": uiPNum, "mono": uiPBool, "tip": uiPText,
}

var uiSizeProps = map[string]uiPropKind{"width": uiPNum, "height": uiPNum, "fill": uiPNum, "anchor": uiPAnchor}

var uiTypeProps = map[string]map[string]uiPropKind{
	"grid":     {"columns": uiPColumns},
	"progress": {"value": uiPFraction},
	"image":    {"src": uiPDataURI},
}

var uiAligns = map[string]bool{"start": true, "center": true, "end": true, "stretch": true}

var uiAnchors = map[string]bool{"fill": true, "center": true, "top": true, "bottom": true, "left": true, "right": true}

const (
	uiArgNone = iota
	uiArgSession
	uiArgGoal
)

var uiActions = map[string]int{
	"open_session": uiArgSession, "new_session": uiArgNone, "focus_input": uiArgNone, "open_palette": uiArgNone,
	"next_waiting": uiArgNone, "show_board": uiArgNone, "open_settings": uiArgNone,
	"allow": uiArgSession, "deny": uiArgSession, "always": uiArgSession, "interrupt": uiArgSession,
	"filter_goal": uiArgGoal, "clear_filter": uiArgNone,
	"stop_session": uiArgSession, "remove_session": uiArgSession, "mark_goal_done": uiArgGoal,
}

var (
	uiIdent  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	uiPath   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	uiHex    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	uiTokRef = regexp.MustCompile(`^\$[A-Za-z][A-Za-z0-9]*$`)
)

func parseUI(raw []byte) (*uiInterface, []string) {
	if len(raw) > uiMaxBytes {
		return nil, []string{fmt.Sprintf("the interface is %d bytes; the limit is %d", len(raw), uiMaxBytes)}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var ui uiInterface
	if err := dec.Decode(&ui); err != nil {
		return nil, []string{"invalid interface JSON: " + strings.TrimPrefix(err.Error(), "json: ")}
	}
	if dec.More() {
		return nil, []string{"invalid interface JSON: extra data after the interface"}
	}
	if errs := ui.check(); len(errs) > 0 {
		return nil, errs
	}
	return &ui, nil
}

func checkUIName(name string, saved map[string]string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("the interface needs a name")
	}
	if strings.EqualFold(strings.TrimSpace(name), uiDefaultName) {
		return fmt.Errorf("%q is reserved for the built-in interface", uiDefaultName)
	}
	if _, ok := saved[name]; !ok && len(saved) >= uiMaxInterfaces {
		return fmt.Errorf("%d interfaces are saved, the limit; delete one first", len(saved))
	}
	return nil
}

func (ui *uiInterface) check() []string {
	c := &uiChecker{}
	if strings.TrimSpace(ui.Name) == "" {
		c.fail("name", "the interface needs a name")
	} else if strings.EqualFold(strings.TrimSpace(ui.Name), uiDefaultName) {
		c.fail("name", fmt.Sprintf("%q is reserved for the built-in interface", uiDefaultName))
	}
	names := make([]string, 0, len(ui.Slots))
	for k := range ui.Slots {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, slot := range names {
		n := ui.Slots[slot]
		at := "slots." + slot
		if !uiSlots[slot] {
			c.fail(at, "unknown slot; slots are header, board, tile, railRow and empty")
			continue
		}
		if n == nil {
			c.fail(at, "the slot is empty")
			continue
		}
		vars := map[string]*uiField{}
		if uiSessionSlots[slot] {
			vars["s"] = uiSessionField
		}
		c.node(at, n, vars)
	}
	return c.errs
}

type uiChecker struct{ errs []string }

func (c *uiChecker) fail(at, msg string) { c.errs = append(c.errs, at+": "+msg) }

func (c *uiChecker) node(at string, n *uiNode, vars map[string]*uiField) {
	if n == nil {
		c.fail(at, "null node")
		return
	}
	native, comp := uiNativeTypes[n.Type], uiComponents[n.Type]
	if !native && !comp {
		c.fail(at+".type", fmt.Sprintf("unknown node type %q", n.Type))
		return
	}
	if n.Each != "" {
		f, err := uiResolve(n.Each, vars)
		switch {
		case err != nil:
			c.fail(at+".each", err.Error())
		case f.kind != uiList:
			c.fail(at+".each", fmt.Sprintf("%s is not a list", n.Each))
		case n.As == "":
			c.fail(at+".as", "each needs as, the name for one item")
		case !uiIdent.MatchString(n.As):
			c.fail(at+".as", fmt.Sprintf("%q is not a plain name", n.As))
		case uiRootField.fields[n.As] != nil || vars[n.As] != nil:
			c.fail(at+".as", fmt.Sprintf("%q is already a data name", n.As))
		default:
			inner := make(map[string]*uiField, len(vars)+1)
			for k, v := range vars {
				inner[k] = v
			}
			inner[n.As] = f.item
			vars = inner
		}
	} else if n.As != "" {
		c.fail(at+".as", "as needs each")
	}
	if n.If != "" {
		if expr, err := parseUICond(n.If); err != nil {
			c.fail(at+".if", err.Error())
		} else if err := expr.check(vars); err != nil {
			c.fail(at+".if", err.Error())
		}
	}
	if n.Text != "" {
		if !uiTextTypes[n.Type] {
			c.fail(at+".text", fmt.Sprintf("%s has no text", n.Type))
		} else if err := uiCheckText(n.Text, vars); err != nil {
			c.fail(at+".text", err.Error())
		}
	}
	if len(n.Kids) > 0 && !uiContainers[n.Type] {
		c.fail(at+".kids", fmt.Sprintf("%s cannot hold kids", n.Type))
	}
	c.props(at+".props", n, vars)
	if n.OnClick != nil {
		c.action(at+".onClick", n.OnClick, vars)
	}
	for i, k := range n.Kids {
		c.node(fmt.Sprintf("%s.kids[%d]", at, i), k, vars)
	}
}

func (c *uiChecker) props(at string, n *uiNode, vars map[string]*uiField) {
	allowed := uiCommonProps
	if uiComponents[n.Type] {
		allowed = uiSizeProps
	}
	keys := make([]string, 0, len(n.Props))
	for k := range n.Props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := n.Props[k]
		kind, ok := allowed[k]
		if !ok {
			kind, ok = uiTypeProps[n.Type][k]
		}
		if !ok && k == "session" && uiComponentNeedsSession[n.Type] {
			kind, ok = uiPText, true
		}
		if !ok {
			c.fail(at+"."+k, fmt.Sprintf("%s has no prop %q", n.Type, k))
			continue
		}
		if err := uiCheckProp(kind, v, vars); err != nil {
			c.fail(at+"."+k, err.Error())
		}
	}
	if uiComponentNeedsSession[n.Type] {
		if s, _ := n.Props["session"].(string); strings.TrimSpace(s) == "" {
			c.fail(at+".session", fmt.Sprintf("%s needs a session", n.Type))
		}
	}
}

func uiCheckProp(kind uiPropKind, v any, vars map[string]*uiField) error {
	if s, ok := v.(string); ok && (kind == uiPNum || kind == uiPBool || kind == uiPColumns || kind == uiPFraction) {
		path, whole := uiWholeBinding(s)
		if !whole {
			return fmt.Errorf("must be a %s or a single {{path}}", uiPropKindName(kind))
		}
		f, err := uiResolve(path, vars)
		if err != nil {
			return err
		}
		want := f.kind == uiNum || f.kind == uiMoney
		if kind == uiPBool {
			want = f.kind == uiBool
		}
		if !want {
			return fmt.Errorf("%s is not a %s", path, uiPropKindName(kind))
		}
		return nil
	}
	switch kind {
	case uiPNum:
		if f, ok := v.(float64); !ok || f < 0 {
			return fmt.Errorf("must be a number of 0 or more")
		}
	case uiPColumns:
		if f, ok := v.(float64); !ok || f < 1 || f != float64(int(f)) {
			return fmt.Errorf("must be a whole number of 1 or more")
		}
	case uiPFraction:
		if f, ok := v.(float64); !ok || f < 0 || f > 1 {
			return fmt.Errorf("must be a number from 0 to 1")
		}
	case uiPBool:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("must be true or false")
		}
	case uiPColour:
		s, _ := v.(string)
		if !uiHex.MatchString(s) && !uiTokRef.MatchString(s) {
			return fmt.Errorf("must be a theme token such as $accent or #rrggbb")
		}
	case uiPAlign:
		if s, _ := v.(string); !uiAligns[s] {
			return fmt.Errorf("must be start, center, end or stretch")
		}
	case uiPAnchor:
		if s, _ := v.(string); !uiAnchors[s] {
			return fmt.Errorf("must be fill, center, top, bottom, left or right")
		}
	case uiPDataURI:
		if s, _ := v.(string); !strings.HasPrefix(s, "data:image/") {
			return fmt.Errorf("must be a data:image/ URI")
		}
	case uiPText:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("must be a string")
		}
		return uiCheckText(s, vars)
	}
	return nil
}

func uiPropKindName(k uiPropKind) string {
	if k == uiPBool {
		return "true or false"
	}
	return "number"
}

func (c *uiChecker) action(at string, a *uiAction, vars map[string]*uiField) {
	arg, ok := uiActions[a.Action]
	if !ok {
		c.fail(at+".action", fmt.Sprintf("unknown action %q", a.Action))
		return
	}
	for _, p := range []struct {
		key  string
		val  string
		want bool
	}{{"session", a.Session, arg == uiArgSession}, {"goal", a.Goal, arg == uiArgGoal}} {
		switch {
		case p.want && strings.TrimSpace(p.val) == "":
			c.fail(at+"."+p.key, fmt.Sprintf("%s needs %s", a.Action, p.key))
		case !p.want && p.val != "":
			c.fail(at+"."+p.key, fmt.Sprintf("%s takes no %s", a.Action, p.key))
		case p.val != "":
			if err := uiCheckText(p.val, vars); err != nil {
				c.fail(at+"."+p.key, err.Error())
			}
		}
	}
}

func uiResolve(path string, vars map[string]*uiField) (*uiField, error) {
	if !uiPath.MatchString(path) {
		return nil, fmt.Errorf("%q is not a path", path)
	}
	parts := strings.Split(path, ".")
	f := vars[parts[0]]
	if f == nil {
		f = uiRootField.fields[parts[0]]
	}
	if f == nil {
		return nil, fmt.Errorf("unknown path %s", path)
	}
	for _, p := range parts[1:] {
		if f.kind != uiObj || f.fields[p] == nil {
			return nil, fmt.Errorf("unknown path %s", path)
		}
		f = f.fields[p]
	}
	return f, nil
}

type uiPiece struct {
	lit  string
	path string
}

func splitUIBindings(s string) ([]uiPiece, error) {
	var out []uiPiece
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			if s != "" {
				out = append(out, uiPiece{lit: s})
			}
			return out, nil
		}
		if i > 0 {
			out = append(out, uiPiece{lit: s[:i]})
		}
		j := strings.Index(s[i+2:], "}}")
		if j < 0 {
			return nil, fmt.Errorf("unclosed {{ in %q", s)
		}
		p := strings.TrimSpace(s[i+2 : i+2+j])
		if !uiPath.MatchString(p) {
			return nil, fmt.Errorf("%q is not a path", p)
		}
		out = append(out, uiPiece{path: p})
		s = s[i+2+j+2:]
	}
}

func uiWholeBinding(s string) (string, bool) {
	ps, err := splitUIBindings(s)
	if err != nil || len(ps) != 1 || ps[0].path == "" {
		return "", false
	}
	return ps[0].path, true
}

func uiCheckText(s string, vars map[string]*uiField) error {
	ps, err := splitUIBindings(s)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.path == "" {
			continue
		}
		f, err := uiResolve(p.path, vars)
		if err != nil {
			return err
		}
		if f.kind == uiList || f.kind == uiObj {
			return fmt.Errorf("%s is a list or group and cannot go in text", p.path)
		}
	}
	return nil
}

type uiCond struct {
	or [][]uiCmp
}

type uiCmp struct {
	not  bool
	path string
	op   string
	num  *float64
	str  *string
}

func parseUICond(src string) (*uiCond, error) {
	toks, err := lexUICond(src)
	if err != nil {
		return nil, err
	}
	c := &uiCond{}
	var and []uiCmp
	for i := 0; ; {
		if i >= len(toks) {
			return nil, fmt.Errorf("condition %q ends early", src)
		}
		var cmp uiCmp
		if toks[i] == "!" {
			cmp.not = true
			i++
			if i >= len(toks) {
				return nil, fmt.Errorf("condition %q ends early", src)
			}
		}
		if !uiPath.MatchString(toks[i]) {
			return nil, fmt.Errorf("condition %q: expected a path, got %s", src, toks[i])
		}
		cmp.path = toks[i]
		i++
		if i < len(toks) && (toks[i] == "==" || toks[i] == "!=" || toks[i] == "<" || toks[i] == ">") {
			if cmp.not {
				return nil, fmt.Errorf("condition %q: ! cannot go before a comparison", src)
			}
			cmp.op = toks[i]
			i++
			if i >= len(toks) {
				return nil, fmt.Errorf("condition %q ends early", src)
			}
			lit := toks[i]
			if strings.HasPrefix(lit, "\"") || strings.HasPrefix(lit, "'") {
				s := lit[1 : len(lit)-1]
				cmp.str = &s
			} else if f, err := strconv.ParseFloat(lit, 64); err == nil {
				cmp.num = &f
			} else {
				return nil, fmt.Errorf("condition %q: compare with a number or a quoted string, not %s", src, lit)
			}
			i++
		}
		and = append(and, cmp)
		if i == len(toks) {
			c.or = append(c.or, and)
			return c, nil
		}
		switch toks[i] {
		case "&&":
		case "||":
			c.or = append(c.or, and)
			and = nil
		default:
			return nil, fmt.Errorf("condition %q: expected && or ||, got %s", src, toks[i])
		}
		i++
	}
}

func lexUICond(src string) ([]string, error) {
	var toks []string
	for i := 0; i < len(src); {
		ch := src[i]
		switch {
		case ch == ' ' || ch == '\t':
			i++
		case ch == '"' || ch == '\'':
			j := strings.IndexByte(src[i+1:], ch)
			if j < 0 {
				return nil, fmt.Errorf("condition %q has an unclosed string", src)
			}
			toks = append(toks, src[i:i+j+2])
			i += j + 2
		case strings.HasPrefix(src[i:], "&&"), strings.HasPrefix(src[i:], "||"),
			strings.HasPrefix(src[i:], "=="), strings.HasPrefix(src[i:], "!="):
			toks = append(toks, src[i:i+2])
			i += 2
		case ch == '!' || ch == '<' || ch == '>':
			toks = append(toks, string(ch))
			i++
		default:
			j := i
			for j < len(src) && strings.IndexByte(" \t\"'&|=!<>", src[j]) < 0 {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("condition %q: unexpected %q", src, ch)
			}
			toks = append(toks, src[i:j])
			i = j
		}
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	return toks, nil
}

func (c *uiCond) check(vars map[string]*uiField) error {
	for _, and := range c.or {
		for _, cmp := range and {
			f, err := uiResolve(cmp.path, vars)
			if err != nil {
				return err
			}
			if cmp.op == "" {
				continue
			}
			isNum := f.kind == uiNum || f.kind == uiMoney
			switch {
			case cmp.num != nil && !isNum:
				return fmt.Errorf("%s is not a number", cmp.path)
			case cmp.str != nil && f.kind != uiStr:
				return fmt.Errorf("%s is not text", cmp.path)
			case cmp.str != nil && (cmp.op == "<" || cmp.op == ">"):
				return fmt.Errorf("%s only compares numbers", cmp.op)
			}
		}
	}
	return nil
}

type uiScope struct {
	root   map[string]any
	parent *uiScope
	name   string
	val    any
	field  *uiField
}

func newUIScope(root map[string]any, session any) *uiScope {
	sc := &uiScope{root: root}
	if session != nil {
		sc = sc.with("s", session, uiSessionField)
	}
	return sc
}

func (sc *uiScope) with(name string, v any, f *uiField) *uiScope {
	return &uiScope{root: sc.root, parent: sc, name: name, val: v, field: f}
}

func (sc *uiScope) lookup(path string) (any, *uiField, bool) {
	parts := strings.Split(path, ".")
	var v any
	var f *uiField
	found := false
	for s := sc; s != nil; s = s.parent {
		if s.name == parts[0] {
			v, f, found = s.val, s.field, true
			break
		}
	}
	if !found {
		v, found = sc.root[parts[0]]
		f = uiRootField.fields[parts[0]]
	}
	if !found {
		return nil, nil, false
	}
	for _, p := range parts[1:] {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		if v, ok = m[p]; !ok {
			return nil, nil, false
		}
		if f != nil && f.fields != nil {
			f = f.fields[p]
		} else {
			f = nil
		}
	}
	if v == nil {
		return nil, nil, false
	}
	return v, f, true
}

func uiNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

func uiItems(v any) ([]any, bool) {
	switch l := v.(type) {
	case []any:
		return l, true
	case []map[string]any:
		out := make([]any, len(l))
		for i, m := range l {
			out[i] = m
		}
		return out, true
	}
	return nil, false
}

func uiFormat(v any, f *uiField) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	}
	if n, ok := uiNumber(v); ok {
		if f != nil && f.kind == uiMoney {
			return strconv.FormatFloat(n, 'f', 2, 64), nil
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("cannot print a %T", v)
}

func (sc *uiScope) interp(s string) (string, error) {
	ps, err := splitUIBindings(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range ps {
		if p.path == "" {
			b.WriteString(p.lit)
			continue
		}
		v, f, ok := sc.lookup(p.path)
		if !ok {
			continue
		}
		out, err := uiFormat(v, f)
		if err != nil {
			return "", fmt.Errorf("%s: %v", p.path, err)
		}
		b.WriteString(out)
	}
	return b.String(), nil
}

func (sc *uiScope) value(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	if path, whole := uiWholeBinding(s); whole {
		got, _, found := sc.lookup(path)
		if !found {
			return nil, nil
		}
		return got, nil
	}
	return sc.interp(s)
}

func (sc *uiScope) action(a *uiAction) (uiAction, error) {
	out := uiAction{Action: a.Action}
	var err error
	if out.Session, err = sc.interp(a.Session); err != nil {
		return out, err
	}
	out.Goal, err = sc.interp(a.Goal)
	return out, err
}

func (sc *uiScope) cond(src string) (bool, error) {
	c, err := parseUICond(src)
	if err != nil {
		return false, err
	}
	for _, and := range c.or {
		all := true
		for _, cmp := range and {
			ok, err := sc.cmp(cmp)
			if err != nil {
				return false, err
			}
			if !ok {
				all = false
				break
			}
		}
		if all {
			return true, nil
		}
	}
	return false, nil
}

func (sc *uiScope) cmp(c uiCmp) (bool, error) {
	v, _, found := sc.lookup(c.path)
	if c.op == "" {
		t := found && uiTruthy(v)
		return t != c.not, nil
	}
	if c.op == "<" || c.op == ">" {
		if !found {
			return false, nil
		}
		n, ok := uiNumber(v)
		if !ok || c.num == nil {
			return false, fmt.Errorf("%s %s compares numbers only", c.path, c.op)
		}
		if c.op == "<" {
			return n < *c.num, nil
		}
		return n > *c.num, nil
	}
	eq := false
	if found {
		if c.num != nil {
			n, ok := uiNumber(v)
			eq = ok && n == *c.num
		} else {
			s, ok := v.(string)
			eq = ok && s == *c.str
		}
	}
	return eq == (c.op == "=="), nil
}

func uiTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	}
	if n, ok := uiNumber(v); ok {
		return n != 0
	}
	if l, ok := uiItems(v); ok {
		return len(l) > 0
	}
	return v != nil
}

func (sc *uiScope) expand(n *uiNode) ([]*uiScope, error) {
	scopes := []*uiScope{sc}
	if n.Each != "" {
		v, f, found := sc.lookup(n.Each)
		scopes = nil
		if found {
			items, ok := uiItems(v)
			if !ok {
				return nil, fmt.Errorf("%s is not a list", n.Each)
			}
			var item *uiField
			if f != nil {
				item = f.item
			}
			for _, it := range items {
				scopes = append(scopes, sc.with(n.As, it, item))
			}
		}
	}
	if n.If == "" {
		return scopes, nil
	}
	kept := scopes[:0:0]
	for _, s := range scopes {
		ok, err := s.cond(n.If)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, s)
		}
	}
	return kept, nil
}
