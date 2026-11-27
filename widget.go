package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	widgetTool       = "show_widget"
	widgetToolFull   = "mcp__switchboard__" + widgetTool
	widgetMaxBytes   = 64 << 10
	widgetMaxIDs     = 50
	widgetMaxRows    = 500
	widgetMaxCols    = 12
	widgetMaxCell    = 200
	widgetMaxPoints  = 200
	widgetMaxSeries  = 2
	widgetMaxFields  = 12
	widgetMaxButtons = 6
	widgetMaxBars    = 12
	widgetMaxItems   = 50
	widgetMaxOptions = 20
	widgetMaxTitle   = 120
	widgetMaxLabel   = 40
	widgetMaxFrag    = 16 << 10
	widgetMaxUniform = 16
	widgetMinShaderH = 40
	widgetMaxShaderH = 600
	widgetShaderH    = 200
)

const widgetTriggers = "Use show_widget on your own, without waiting to be asked. " +
	"Show a table for any result with several rows of the same fields, such as files changed, test results, findings or timings, instead of writing a markdown table. " +
	"Show progress for work with more than about three steps, and as each step finishes call show_widget again with the same id to update it in place. " +
	"Show a chart for numbers compared across items or over time. " +
	"Show a keyvalue list for a short status summary at the end of a turn. " +
	"Still write your answer in prose; the widget goes alongside it, not instead of it. " +
	"Ask the user questions with AskUserQuestion, never with a buttons or form widget."

const sessionPrompt = "Switchboard shows this conversation in a window that renders widgets from the show_widget tool. " + widgetTriggers

type Widget struct {
	ID       string     `json:"id,omitempty"`
	Kind     string     `json:"kind"`
	Title    string     `json:"title,omitempty"`
	Tone     string     `json:"tone,omitempty"`
	Table    *WTable    `json:"table,omitempty"`
	Progress *WProgress `json:"progress,omitempty"`
	KeyValue *WKeyValue `json:"keyvalue,omitempty"`
	Chart    *WChart    `json:"chart,omitempty"`
	Buttons  *WButtons  `json:"buttons,omitempty"`
	Form     *WForm     `json:"form,omitempty"`
	Shader   *WShader   `json:"shader,omitempty"`
}

type WColumn struct {
	Title string `json:"title"`
	Mono  bool   `json:"mono,omitempty"`
	Align string `json:"align,omitempty"`
}

type WRow struct {
	Cells []string `json:"cells"`
	Tone  string   `json:"tone,omitempty"`
}

type WTable struct {
	Columns []WColumn `json:"columns"`
	Rows    []WRow    `json:"rows"`
}

type WBar struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Tone  string  `json:"tone,omitempty"`
}

type WProgress struct {
	Bars []WBar `json:"bars"`
}

type WItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

type WKeyValue struct {
	Items []WItem `json:"items"`
}

type WSeries struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
	Tone   string    `json:"tone,omitempty"`
}

type WChart struct {
	Type   string    `json:"type"`
	Labels []string  `json:"labels,omitempty"`
	Unit   string    `json:"unit,omitempty"`
	Series []WSeries `json:"series"`
}

type WButton struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

type WButtons struct {
	Prompt  string    `json:"prompt,omitempty"`
	Buttons []WButton `json:"buttons"`
}

type WField struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	Rows        int      `json:"rows,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

type WForm struct {
	Fields      []WField `json:"fields"`
	SubmitLabel string   `json:"submit_label,omitempty"`
}

type WShader struct {
	Frag     string             `json:"frag"`
	Uniforms map[string]float64 `json:"uniforms,omitempty"`
	Height   float64            `json:"height,omitempty"`
	Animate  bool               `json:"animate,omitempty"`
}

type WidgetAnswer struct {
	Clicked string            `json:"clicked,omitempty"`
	Values  map[string]string `json:"values,omitempty"`
}

var (
	widgetIDRE    = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
	fieldNameRE   = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)
	uniformNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,39}$`)
	widgetEventRE = regexp.MustCompile(`(?s)<switchboard-widget id="([a-z0-9_-]{1,40})" kind="([a-z]+)">(.*?)</switchboard-widget>`)
	widgetKinds   = []string{"table", "progress", "keyvalue", "chart", "buttons", "form", "shader"}
	widgetTones   = []string{"", "neutral", "good", "warn", "bad"}
)

func (w *Widget) interactive() bool { return w.Kind == "buttons" || w.Kind == "form" }

func parseWidget(args map[string]any) (*Widget, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	if len(raw) > widgetMaxBytes {
		return nil, fmt.Errorf("the widget is %d KB; the limit is %d KB", len(raw)>>10, widgetMaxBytes>>10)
	}
	var w Widget
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("the widget does not match the schema: %v", err)
	}
	if err := w.check(); err != nil {
		return nil, err
	}
	return &w, nil
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func longer(s string, n int) bool { return len([]rune(s)) > n }

func checkTone(where, t string) error {
	if !slices.Contains(widgetTones, t) {
		return fmt.Errorf("%s: tone %q is not one of neutral, good, warn, bad", where, t)
	}
	return nil
}

func (w *Widget) check() error {
	if w.ID != "" && !widgetIDRE.MatchString(w.ID) {
		return fmt.Errorf("id %q must be 1 to 40 of a-z, 0-9, _ and -", w.ID)
	}
	if !slices.Contains(widgetKinds, w.Kind) {
		return fmt.Errorf("kind %q is not one of %s", w.Kind, strings.Join(widgetKinds, ", "))
	}
	if longer(w.Title, widgetMaxTitle) {
		return fmt.Errorf("title is longer than %d characters", widgetMaxTitle)
	}
	if err := checkTone("widget", w.Tone); err != nil {
		return err
	}
	missing := fmt.Errorf("kind %s needs a %s object", w.Kind, w.Kind)
	switch w.Kind {
	case "table":
		if w.Table == nil {
			return missing
		}
		return w.Table.check()
	case "progress":
		if w.Progress == nil {
			return missing
		}
		return w.Progress.check()
	case "keyvalue":
		if w.KeyValue == nil {
			return missing
		}
		return w.KeyValue.check()
	case "chart":
		if w.Chart == nil {
			return missing
		}
		return w.Chart.check()
	case "buttons":
		if w.Buttons == nil {
			return missing
		}
		return w.Buttons.check()
	case "form":
		if w.Form == nil {
			return missing
		}
		return w.Form.check()
	case "shader":
		if w.Shader == nil {
			return missing
		}
		return w.Shader.check()
	}
	return nil
}

func (s *WShader) check() error {
	if strings.TrimSpace(s.Frag) == "" {
		return fmt.Errorf("shader: frag is empty")
	}
	if len(s.Frag) > widgetMaxFrag {
		return fmt.Errorf("shader: frag is %d bytes; the limit is %d", len(s.Frag), widgetMaxFrag)
	}
	if len(s.Uniforms) > widgetMaxUniform {
		return fmt.Errorf("shader: %d uniforms; the limit is %d", len(s.Uniforms), widgetMaxUniform)
	}
	for name := range s.Uniforms {
		if !uniformNameRE.MatchString(name) {
			return fmt.Errorf("shader: uniform %q must be a GLSL name of up to 40 letters, digits and _, not starting with a digit", name)
		}
		if strings.HasPrefix(name, "u_") || strings.HasPrefix(name, "gl_") {
			return fmt.Errorf("shader: uniform %q uses a reserved prefix, u_ or gl_", name)
		}
	}
	if s.Height == 0 {
		s.Height = widgetShaderH
	}
	if s.Height < widgetMinShaderH || s.Height > widgetMaxShaderH {
		return fmt.Errorf("shader: height %v is outside %d to %d", s.Height, widgetMinShaderH, widgetMaxShaderH)
	}
	return nil
}

func (t *WTable) check() error {
	if len(t.Columns) == 0 || len(t.Columns) > widgetMaxCols {
		return fmt.Errorf("table: needs 1 to %d columns, got %d", widgetMaxCols, len(t.Columns))
	}
	if len(t.Rows) > widgetMaxRows {
		return fmt.Errorf("table: %d rows; the limit is %d", len(t.Rows), widgetMaxRows)
	}
	for i := range t.Columns {
		c := &t.Columns[i]
		c.Title = clipRunes(c.Title, widgetMaxCell)
		if c.Align != "" && c.Align != "left" && c.Align != "right" {
			return fmt.Errorf("table: column %d align %q is not left or right", i+1, c.Align)
		}
	}
	for i := range t.Rows {
		r := &t.Rows[i]
		if len(r.Cells) > len(t.Columns) {
			return fmt.Errorf("table: row %d has %d cells for %d columns", i+1, len(r.Cells), len(t.Columns))
		}
		if err := checkTone(fmt.Sprintf("table row %d", i+1), r.Tone); err != nil {
			return err
		}
		for len(r.Cells) < len(t.Columns) {
			r.Cells = append(r.Cells, "")
		}
		for j := range r.Cells {
			r.Cells[j] = clipRunes(r.Cells[j], widgetMaxCell)
		}
	}
	return nil
}

func (p *WProgress) check() error {
	if len(p.Bars) == 0 || len(p.Bars) > widgetMaxBars {
		return fmt.Errorf("progress: needs 1 to %d bars, got %d", widgetMaxBars, len(p.Bars))
	}
	for i := range p.Bars {
		b := &p.Bars[i]
		if b.Value < 0 || b.Value > 1 {
			return fmt.Errorf("progress: bar %d value %v is outside 0 to 1", i+1, b.Value)
		}
		if err := checkTone(fmt.Sprintf("progress bar %d", i+1), b.Tone); err != nil {
			return err
		}
		b.Label = clipRunes(b.Label, widgetMaxCell)
	}
	return nil
}

func (k *WKeyValue) check() error {
	if len(k.Items) == 0 || len(k.Items) > widgetMaxItems {
		return fmt.Errorf("keyvalue: needs 1 to %d items, got %d", widgetMaxItems, len(k.Items))
	}
	for i := range k.Items {
		it := &k.Items[i]
		if err := checkTone(fmt.Sprintf("keyvalue item %d", i+1), it.Tone); err != nil {
			return err
		}
		it.Key, it.Value = clipRunes(it.Key, widgetMaxCell), clipRunes(it.Value, widgetMaxCell)
	}
	return nil
}

func (c *WChart) check() error {
	if c.Type != "bar" && c.Type != "line" {
		return fmt.Errorf("chart: type %q is not bar or line", c.Type)
	}
	if len(c.Series) == 0 || len(c.Series) > widgetMaxSeries {
		return fmt.Errorf("chart: needs 1 to %d series, got %d", widgetMaxSeries, len(c.Series))
	}
	if len(c.Labels) > widgetMaxPoints {
		return fmt.Errorf("chart: %d labels; the limit is %d", len(c.Labels), widgetMaxPoints)
	}
	n := len(c.Series[0].Values)
	for i := range c.Series {
		s := &c.Series[i]
		if len(s.Values) == 0 || len(s.Values) > widgetMaxPoints {
			return fmt.Errorf("chart: series %d needs 1 to %d values, got %d", i+1, widgetMaxPoints, len(s.Values))
		}
		if len(s.Values) != n {
			return fmt.Errorf("chart: every series needs the same number of values")
		}
		if err := checkTone(fmt.Sprintf("chart series %d", i+1), s.Tone); err != nil {
			return err
		}
		s.Name = clipRunes(s.Name, widgetMaxLabel)
	}
	if len(c.Labels) > 0 && len(c.Labels) != n {
		return fmt.Errorf("chart: %d labels for %d values", len(c.Labels), n)
	}
	for i := range c.Labels {
		c.Labels[i] = clipRunes(c.Labels[i], widgetMaxLabel)
	}
	c.Unit = clipRunes(c.Unit, widgetMaxLabel)
	return nil
}

func (b *WButtons) check() error {
	if len(b.Buttons) == 0 || len(b.Buttons) > widgetMaxButtons {
		return fmt.Errorf("buttons: needs 1 to %d buttons, got %d", widgetMaxButtons, len(b.Buttons))
	}
	seen := map[string]bool{}
	for i, x := range b.Buttons {
		l := strings.TrimSpace(x.Label)
		if l == "" || longer(l, widgetMaxLabel) {
			return fmt.Errorf("buttons: button %d label must be 1 to %d characters", i+1, widgetMaxLabel)
		}
		if seen[l] {
			return fmt.Errorf("buttons: two buttons are labelled %q", l)
		}
		seen[l] = true
		if err := checkTone(fmt.Sprintf("button %d", i+1), x.Tone); err != nil {
			return err
		}
		b.Buttons[i].Label = l
	}
	b.Prompt = clipRunes(b.Prompt, widgetMaxCell)
	return nil
}

func (f *WForm) check() error {
	if len(f.Fields) == 0 || len(f.Fields) > widgetMaxFields {
		return fmt.Errorf("form: needs 1 to %d fields, got %d", widgetMaxFields, len(f.Fields))
	}
	if longer(f.SubmitLabel, widgetMaxLabel) {
		return fmt.Errorf("form: submit_label is longer than %d characters", widgetMaxLabel)
	}
	seen := map[string]bool{}
	for i := range f.Fields {
		x := &f.Fields[i]
		if !fieldNameRE.MatchString(x.Name) {
			return fmt.Errorf("form: field %d name %q must be 1 to 40 of letters, digits and _", i+1, x.Name)
		}
		if seen[x.Name] {
			return fmt.Errorf("form: two fields are named %q", x.Name)
		}
		seen[x.Name] = true
		x.Label = clipRunes(x.Label, widgetMaxCell)
		x.Placeholder = clipRunes(x.Placeholder, widgetMaxCell)
		switch x.Kind {
		case "text", "textarea":
			if len(x.Options) > 0 {
				return fmt.Errorf("form: field %s is %s and cannot have options", x.Name, x.Kind)
			}
			if x.Rows == 0 {
				x.Rows = 3
			}
			x.Rows = min(max(x.Rows, 2), 10)
		case "choice":
			if len(x.Options) == 0 || len(x.Options) > widgetMaxOptions {
				return fmt.Errorf("form: field %s needs 1 to %d options", x.Name, widgetMaxOptions)
			}
			for j := range x.Options {
				x.Options[j] = clipRunes(x.Options[j], widgetMaxLabel)
			}
			if x.Default != "" && !slices.Contains(x.Options, x.Default) {
				return fmt.Errorf("form: field %s default %q is not one of its options", x.Name, x.Default)
			}
		default:
			return fmt.Errorf("form: field %s kind %q is not text, textarea or choice", x.Name, x.Kind)
		}
	}
	return nil
}

func widgetEvent(w *Widget, ans WidgetAnswer) string {
	b, _ := json.Marshal(ans)
	return fmt.Sprintf(`<switchboard-widget id="%s" kind="%s">%s</switchboard-widget>`, w.ID, w.Kind, b)
}

type widgetReply struct {
	id  string
	ans WidgetAnswer
}

func widgetReplies(text string) ([]widgetReply, string) {
	var out []widgetReply
	for _, m := range widgetEventRE.FindAllStringSubmatch(text, -1) {
		var ans WidgetAnswer
		if json.Unmarshal([]byte(m[3]), &ans) == nil {
			out = append(out, widgetReply{id: m[1], ans: ans})
		}
	}
	return out, strings.TrimSpace(widgetEventRE.ReplaceAllString(text, ""))
}

func widgetEventLabel(text string) string {
	rs, _ := widgetReplies(text)
	if len(rs) == 0 {
		return text
	}
	if c := rs[0].ans.Clicked; c != "" {
		return "Clicked “" + c + "”"
	}
	return "Submitted a form"
}

func widgetToolResult(text string) (id string, ok bool) {
	for _, verb := range []string{"shown ", "updated "} {
		if s, found := strings.CutPrefix(strings.TrimSpace(text), verb); found && widgetIDRE.MatchString(s) {
			return s, true
		}
	}
	return "", false
}

func (a *Agent) ShowWidget(args map[string]any) (string, error) {
	w, err := parseWidget(args)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	res, err := a.putWidget(w, true)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	a.changed()
	return res, nil
}

func (a *Agent) putWidget(w *Widget, check bool) (string, error) {
	if a.widgets == nil {
		a.widgets = map[string]int{}
	}
	if w.ID == "" {
		for n := len(a.widgets) + 1; ; n++ {
			if id := fmt.Sprintf("w%d", n); !a.hasWidget(id) {
				w.ID = id
				break
			}
		}
	}
	if a.hasWidget(w.ID) {
		e := &a.Entries[a.widgets[w.ID]]
		if check && e.Answer != nil {
			return "", fmt.Errorf("widget %s was answered, so it cannot change; show a new widget with another id", w.ID)
		}
		e.Widget, e.Answer, e.Tool = w, nil, w.Kind
		e.Rev++
		return "updated " + w.ID, nil
	}
	if check && len(a.widgets) >= widgetMaxIDs {
		return "", fmt.Errorf("this conversation already has %d widgets, the limit; update one by its id instead", widgetMaxIDs)
	}
	a.widgets[w.ID] = a.addEntry(Entry{Kind: EntryWidget, Tool: w.Kind, Text: w.Title, Widget: w})
	return "shown " + w.ID, nil
}

func (a *Agent) hasWidget(id string) bool {
	i, ok := a.widgets[id]
	return ok && i < len(a.Entries) && a.Entries[i].Kind == EntryWidget
}

func (a *Agent) setAnswer(id string, ans *WidgetAnswer) bool {
	if !a.hasWidget(id) {
		return false
	}
	e := &a.Entries[a.widgets[id]]
	if !e.Widget.interactive() {
		return false
	}
	e.Answer = ans
	e.Rev++
	return true
}

func (a *Agent) unanswerWidgets(text string) {
	rs, _ := widgetReplies(text)
	for _, r := range rs {
		a.setAnswer(r.id, nil)
	}
}

func (a *Agent) replayWidgetReplies(text string) string {
	rs, rest := widgetReplies(text)
	for _, r := range rs {
		a.setAnswer(r.id, &r.ans)
	}
	return rest
}

func (a *Agent) AnswerWidget(id string, ans WidgetAnswer) {
	a.mu.Lock()
	if !a.hasWidget(id) || a.Entries[a.widgets[id]].Answer != nil {
		a.mu.Unlock()
		return
	}
	a.setAnswer(id, &ans)
	a.enqueue(widgetEvent(a.Entries[a.widgets[id]].Widget, ans), "widget")
	hold := a.busy() || a.editing != 0 || a.State == NeedsYou
	a.mu.Unlock()
	a.changed()
	if !hold {
		a.SendFrom("", "")
	}
}

func (a *Agent) widgetUse(id string, input map[string]any) {
	if a.widgetUses == nil {
		a.widgetUses = map[string]map[string]any{}
	}
	a.widgetUses[id] = input
	a.Activity = "Showing a widget"
	a.live()
}

func (a *Agent) widgetResult(id, text string, failed bool) bool {
	input, ok := a.widgetUses[id]
	if !ok {
		return false
	}
	delete(a.widgetUses, id)
	if a.State == Working {
		a.Activity = "Thinking"
	}
	if !a.replaying || failed {
		return true
	}
	wid, ok := widgetToolResult(text)
	if !ok {
		return true
	}
	if w, err := parseWidget(input); err == nil {
		w.ID = wid
		_, _ = a.putWidget(w, false)
	}
	return true
}

func widgetSummary(e Entry) string {
	s := e.Widget.Kind + " " + e.Widget.ID
	if e.Widget.Title != "" {
		s += ": " + e.Widget.Title
	}
	if e.Answer != nil {
		b, _ := json.Marshal(e.Answer)
		s += ", answered " + string(b)
	}
	return s
}

var widgetToolDef = func() map[string]any {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	enum := func(d string, vs ...string) map[string]any {
		return map[string]any{"type": "string", "enum": vs, "description": d}
	}
	arr := func(items map[string]any, max int, d string) map[string]any {
		return map[string]any{"type": "array", "items": items, "maxItems": max, "description": d}
	}
	tone := enum("Color: neutral, good, warn or bad. Leave out for neutral.", "neutral", "good", "warn", "bad")
	num := map[string]any{"type": "number"}
	return map[string]any{
		"name": widgetTool,
		"description": "Show a widget in this conversation, in Switchboard's window: a table, progress bars, a keyvalue list, a bar or line chart, a row of buttons, a short form, or a fragment shader. " +
			widgetTriggers + " " +
			"Text in widgets is plain; markdown and links are not rendered. " +
			"Returns \"shown <id>\" or \"updated <id>\". Call it again with the same id to replace that widget in place, for example to advance progress bars. " +
			"Updates to one widget show at most 4 times a second, and a shader widget's animate runs on the GPU between updates, so animate it with u_time rather than repeated calls. A conversation holds at most 50 widgets. " +
			"When the user clicks a button or submits a form, the widget locks and you get a message <switchboard-widget id=\"<id>\" kind=\"buttons\">{\"clicked\":\"<label>\"}</switchboard-widget>, " +
			"or {\"values\":{\"<field name>\":\"<value>\"}} for a form. An answered widget cannot be changed; show a new one. Fill in only the object named by kind.",
		"inputSchema": obj(map[string]any{
			"id":    str("1 to 40 of a-z, 0-9, _ and -. Reuse an id to replace that widget. Leave out for a new one."),
			"kind":  enum("Which widget.", widgetKinds...),
			"title": str("A heading, up to 120 characters."),
			"tone":  tone,
			"table": obj(map[string]any{
				"columns": arr(obj(map[string]any{"title": str("Header."), "mono": map[string]any{"type": "boolean"}, "align": enum("Cell alignment.", "left", "right")}, "title"), widgetMaxCols, "Up to 12 columns."),
				"rows":    arr(obj(map[string]any{"cells": arr(map[string]any{"type": "string"}, widgetMaxCols, "One string per column, up to 200 characters each."), "tone": tone}, "cells"), widgetMaxRows, "Up to 500 rows."),
			}, "columns", "rows"),
			"progress": obj(map[string]any{
				"bars": arr(obj(map[string]any{"label": str("What it measures."), "value": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "tone": tone}, "label", "value"), widgetMaxBars, "Up to 12 bars, each 0 to 1."),
			}, "bars"),
			"keyvalue": obj(map[string]any{
				"items": arr(obj(map[string]any{"key": str(""), "value": str(""), "tone": tone}, "key", "value"), widgetMaxItems, "Up to 50 pairs."),
			}, "items"),
			"chart": obj(map[string]any{
				"type":   enum("bar draws a horizontal bar per label; line plots values left to right.", "bar", "line"),
				"labels": arr(map[string]any{"type": "string"}, widgetMaxPoints, "One label per value."),
				"unit":   str("Shown after values, for example ms or %."),
				"series": arr(obj(map[string]any{"name": str(""), "values": arr(num, widgetMaxPoints, "Up to 200 numbers."), "tone": tone}, "values"), widgetMaxSeries, "One or two series of the same length."),
			}, "type", "series"),
			"buttons": obj(map[string]any{
				"prompt":  str("A line above the buttons."),
				"buttons": arr(obj(map[string]any{"label": str("Up to 40 characters, unique."), "tone": tone}, "label"), widgetMaxButtons, "Up to 6 buttons."),
			}, "buttons"),
			"form": obj(map[string]any{
				"fields": arr(obj(map[string]any{
					"name":        str("Key in the reply: letters, digits and _."),
					"label":       str("Shown beside the field."),
					"kind":        enum("text is one line, textarea several, choice picks one of options.", "text", "textarea", "choice"),
					"placeholder": str(""),
					"default":     str("Starting value; for choice, one of options."),
					"options":     arr(map[string]any{"type": "string"}, widgetMaxOptions, "For choice."),
					"rows":        map[string]any{"type": "integer", "description": "Lines for textarea, 2 to 10."},
					"required":    map[string]any{"type": "boolean"},
				}, "name", "label", "kind"), widgetMaxFields, "Up to 12 fields."),
				"submit_label": str("Label of the submit button. Defaults to Submit."),
			}, "fields"),
			"shader": obj(map[string]any{
				"frag": str("GLSL fragment shader body, up to 16 KB, without #version or precision lines. The desktop app compiles it as GLSL 4.50 and the browser view as GLSL ES 3.00, so use only what both accept. It must define vec4 effect(vec2 uv), which returns the RGBA color for uv, where uv runs 0 to 1 across the widget with its origin at the top left. " +
					"Built-in uniforms: float u_time, seconds since the window started, and vec2 u_res, the widget's size in logical pixels. Each name in uniforms is declared for you as uniform float <name>; do not declare it again. " +
					"No textures or images are available. A shader that fails to compile renders as a blank area and no error is reported back, so keep it simple and check the GLSL yourself."),
				"uniforms": map[string]any{"type": "object", "additionalProperties": num, "maxProperties": widgetMaxUniform,
					"description": "Up to 16 float uniforms by name: letters, digits and _, not starting with a digit, u_ or gl_. Update them by calling again with the same id."},
				"height":  map[string]any{"type": "number", "minimum": widgetMinShaderH, "maximum": widgetMaxShaderH, "description": "Height in logical pixels, 40 to 600. Defaults to 200. The width is the conversation's width."},
				"animate": map[string]any{"type": "boolean", "description": "Redraw every frame while visible, for shaders that use u_time. Leave out for a still image."},
			}, "frag"),
		}, "kind"),
	}
}()
