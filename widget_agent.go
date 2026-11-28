package main

import (
	"encoding/json"
	"fmt"

	"switchboard/widget"
)

const (
	widgetTool     = "show_widget"
	widgetToolFull = "mcp__switchboard__" + widgetTool
)

const widgetTriggers = "Use show_widget on your own, without waiting to be asked. " +
	"Show a table for any result with several rows of the same fields, such as files changed, test results, findings or timings, instead of writing a markdown table. " +
	"Show progress for work with more than about three steps, and as each step finishes call show_widget again with the same id to update it in place. " +
	"Show a chart for numbers compared across items or over time. " +
	"Show a keyvalue list for a short status summary at the end of a turn. " +
	"Still write your answer in prose; the widget goes alongside it, not instead of it. " +
	"Ask the user questions with AskUserQuestion, never with a buttons or form widget."

const sessionPrompt = "Switchboard shows this conversation in a window that renders widgets from the show_widget tool. " + widgetTriggers

func (a *Agent) ShowWidget(args map[string]any) (string, error) {
	w, err := widget.Parse(args)
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

func (a *Agent) putWidget(w *widget.Widget, check bool) (string, error) {
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
	if check && len(a.widgets) >= widget.MaxIDs {
		return "", fmt.Errorf("this conversation already has %d widgets, the limit; update one by its id instead", widget.MaxIDs)
	}
	a.widgets[w.ID] = a.addEntry(Entry{Kind: EntryWidget, Tool: w.Kind, Text: w.Title, Widget: w})
	return "shown " + w.ID, nil
}

func (a *Agent) hasWidget(id string) bool {
	i, ok := a.widgets[id]
	return ok && i < len(a.Entries) && a.Entries[i].Kind == EntryWidget
}

func (a *Agent) setAnswer(id string, ans *widget.Answer) bool {
	if !a.hasWidget(id) {
		return false
	}
	e := &a.Entries[a.widgets[id]]
	if !e.Widget.Interactive() {
		return false
	}
	e.Answer = ans
	e.Rev++
	return true
}

func (a *Agent) unanswerWidgets(text string) {
	rs, _ := widget.Replies(text)
	for _, r := range rs {
		a.setAnswer(r.ID, nil)
	}
}

func (a *Agent) replayWidgetReplies(text string) string {
	rs, rest := widget.Replies(text)
	for _, r := range rs {
		a.setAnswer(r.ID, &r.Answer)
	}
	return rest
}

func (a *Agent) AnswerWidget(id string, ans widget.Answer) {
	a.mu.Lock()
	if !a.hasWidget(id) || a.Entries[a.widgets[id]].Answer != nil {
		a.mu.Unlock()
		return
	}
	a.setAnswer(id, &ans)
	a.enqueue(widget.Event(a.Entries[a.widgets[id]].Widget, ans), "widget")
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
	wid, ok := widget.ToolResult(text)
	if !ok {
		return true
	}
	if w, err := widget.Parse(input); err == nil {
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
			"kind":  enum("Which widget.", widget.Kinds...),
			"title": str("A heading, up to 120 characters."),
			"tone":  tone,
			"table": obj(map[string]any{
				"columns": arr(obj(map[string]any{"title": str("Header."), "mono": map[string]any{"type": "boolean"}, "align": enum("Cell alignment.", "left", "right")}, "title"), widget.MaxCols, "Up to 12 columns."),
				"rows":    arr(obj(map[string]any{"cells": arr(map[string]any{"type": "string"}, widget.MaxCols, "One string per column, up to 200 characters each."), "tone": tone}, "cells"), widget.MaxRows, "Up to 500 rows."),
			}, "columns", "rows"),
			"progress": obj(map[string]any{
				"bars": arr(obj(map[string]any{"label": str("What it measures."), "value": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "tone": tone}, "label", "value"), widget.MaxBars, "Up to 12 bars, each 0 to 1."),
			}, "bars"),
			"keyvalue": obj(map[string]any{
				"items": arr(obj(map[string]any{"key": str(""), "value": str(""), "tone": tone}, "key", "value"), widget.MaxItems, "Up to 50 pairs."),
			}, "items"),
			"chart": obj(map[string]any{
				"type":   enum("bar draws a horizontal bar per label; line plots values left to right.", "bar", "line"),
				"labels": arr(map[string]any{"type": "string"}, widget.MaxPoints, "One label per value."),
				"unit":   str("Shown after values, for example ms or %."),
				"series": arr(obj(map[string]any{"name": str(""), "values": arr(num, widget.MaxPoints, "Up to 200 numbers."), "tone": tone}, "values"), widget.MaxSeries, "One or two series of the same length."),
			}, "type", "series"),
			"buttons": obj(map[string]any{
				"prompt":  str("A line above the buttons."),
				"buttons": arr(obj(map[string]any{"label": str("Up to 40 characters, unique."), "tone": tone}, "label"), widget.MaxButtons, "Up to 6 buttons."),
			}, "buttons"),
			"form": obj(map[string]any{
				"fields": arr(obj(map[string]any{
					"name":        str("Key in the reply: letters, digits and _."),
					"label":       str("Shown beside the field."),
					"kind":        enum("text is one line, textarea several, choice picks one of options.", "text", "textarea", "choice"),
					"placeholder": str(""),
					"default":     str("Starting value; for choice, one of options."),
					"options":     arr(map[string]any{"type": "string"}, widget.MaxOptions, "For choice."),
					"rows":        map[string]any{"type": "integer", "description": "Lines for textarea, 2 to 10."},
					"required":    map[string]any{"type": "boolean"},
				}, "name", "label", "kind"), widget.MaxFields, "Up to 12 fields."),
				"submit_label": str("Label of the submit button. Defaults to Submit."),
			}, "fields"),
			"shader": obj(map[string]any{
				"frag": str("GLSL fragment shader body, up to 16 KB, without #version or precision lines. The desktop app compiles it as GLSL 4.50 and the browser view as GLSL ES 3.00, so use only what both accept. It must define vec4 effect(vec2 uv), which returns the RGBA color for uv, where uv runs 0 to 1 across the widget with its origin at the top left. " +
					"Built-in uniforms: float u_time, seconds since the window started, and vec2 u_res, the widget's size in logical pixels. Each name in uniforms is declared for you as uniform float <name>; do not declare it again. " +
					"No textures or images are available. A shader that fails to compile renders as a blank area and no error is reported back, so keep it simple and check the GLSL yourself."),
				"uniforms": map[string]any{"type": "object", "additionalProperties": num, "maxProperties": widget.MaxUniform,
					"description": "Up to 16 float uniforms by name: letters, digits and _, not starting with a digit, u_ or gl_. Update them by calling again with the same id."},
				"height":  map[string]any{"type": "number", "minimum": widget.MinShaderH, "maximum": widget.MaxShaderH, "description": "Height in logical pixels, 40 to 600. Defaults to 200. The width is the conversation's width."},
				"animate": map[string]any{"type": "boolean", "description": "Redraw every frame while visible, for shaders that use u_time. Leave out for a still image."},
			}, "frag"),
		}, "kind"),
	}
}()
