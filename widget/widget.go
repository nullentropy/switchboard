package widget

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	maxBytes   = 64 << 10
	MaxIDs     = 50
	MaxRows    = 500
	MaxCols    = 12
	maxCell    = 200
	MaxPoints  = 200
	MaxSeries  = 2
	MaxFields  = 12
	MaxButtons = 6
	MaxBars    = 12
	MaxItems   = 50
	MaxOptions = 20
	maxTitle   = 120
	maxLabel   = 40
	maxFrag    = 16 << 10
	MaxUniform = 16
	MinShaderH = 40
	MaxShaderH = 600
	shaderH    = 200
)

type Widget struct {
	ID       string    `json:"id,omitempty"`
	Kind     string    `json:"kind"`
	Title    string    `json:"title,omitempty"`
	Tone     string    `json:"tone,omitempty"`
	Table    *Table    `json:"table,omitempty"`
	Progress *Progress `json:"progress,omitempty"`
	KeyValue *KeyValue `json:"keyvalue,omitempty"`
	Chart    *Chart    `json:"chart,omitempty"`
	Buttons  *Buttons  `json:"buttons,omitempty"`
	Form     *Form     `json:"form,omitempty"`
	Shader   *Shader   `json:"shader,omitempty"`
}

type Column struct {
	Title string `json:"title"`
	Mono  bool   `json:"mono,omitempty"`
	Align string `json:"align,omitempty"`
}

type Row struct {
	Cells []string `json:"cells"`
	Tone  string   `json:"tone,omitempty"`
}

type Table struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
}

type Bar struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Tone  string  `json:"tone,omitempty"`
}

type Progress struct {
	Bars []Bar `json:"bars"`
}

type Item struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

type KeyValue struct {
	Items []Item `json:"items"`
}

type Series struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
	Tone   string    `json:"tone,omitempty"`
}

type Chart struct {
	Type   string   `json:"type"`
	Labels []string `json:"labels,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Series []Series `json:"series"`
}

type Button struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

type Buttons struct {
	Prompt  string   `json:"prompt,omitempty"`
	Buttons []Button `json:"buttons"`
}

type Field struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	Rows        int      `json:"rows,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

type Form struct {
	Fields      []Field `json:"fields"`
	SubmitLabel string  `json:"submit_label,omitempty"`
}

type Shader struct {
	Frag     string             `json:"frag"`
	Uniforms map[string]float64 `json:"uniforms,omitempty"`
	Height   float64            `json:"height,omitempty"`
	Animate  bool               `json:"animate,omitempty"`
}

type Answer struct {
	Clicked string            `json:"clicked,omitempty"`
	Values  map[string]string `json:"values,omitempty"`
}

var (
	idRE          = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
	fieldNameRE   = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)
	uniformNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,39}$`)
	eventRE       = regexp.MustCompile(`(?s)<switchboard-widget id="([a-z0-9_-]{1,40})" kind="([a-z]+)">(.*?)</switchboard-widget>`)
	Kinds         = []string{"table", "progress", "keyvalue", "chart", "buttons", "form", "shader"}
	tones         = []string{"", "neutral", "good", "warn", "bad"}
)

func (w *Widget) Interactive() bool { return w.Kind == "buttons" || w.Kind == "form" }

func Parse(args map[string]any) (*Widget, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, fmt.Errorf("the widget is %d KB; the limit is %d KB", len(raw)>>10, maxBytes>>10)
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
	if !slices.Contains(tones, t) {
		return fmt.Errorf("%s: tone %q is not one of neutral, good, warn, bad", where, t)
	}
	return nil
}

func (w *Widget) check() error {
	if w.ID != "" && !idRE.MatchString(w.ID) {
		return fmt.Errorf("id %q must be 1 to 40 of a-z, 0-9, _ and -", w.ID)
	}
	if !slices.Contains(Kinds, w.Kind) {
		return fmt.Errorf("kind %q is not one of %s", w.Kind, strings.Join(Kinds, ", "))
	}
	if longer(w.Title, maxTitle) {
		return fmt.Errorf("title is longer than %d characters", maxTitle)
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

func (s *Shader) check() error {
	if strings.TrimSpace(s.Frag) == "" {
		return fmt.Errorf("shader: frag is empty")
	}
	if len(s.Frag) > maxFrag {
		return fmt.Errorf("shader: frag is %d bytes; the limit is %d", len(s.Frag), maxFrag)
	}
	if len(s.Uniforms) > MaxUniform {
		return fmt.Errorf("shader: %d uniforms; the limit is %d", len(s.Uniforms), MaxUniform)
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
		s.Height = shaderH
	}
	if s.Height < MinShaderH || s.Height > MaxShaderH {
		return fmt.Errorf("shader: height %v is outside %d to %d", s.Height, MinShaderH, MaxShaderH)
	}
	return nil
}

func (t *Table) check() error {
	if len(t.Columns) == 0 || len(t.Columns) > MaxCols {
		return fmt.Errorf("table: needs 1 to %d columns, got %d", MaxCols, len(t.Columns))
	}
	if len(t.Rows) > MaxRows {
		return fmt.Errorf("table: %d rows; the limit is %d", len(t.Rows), MaxRows)
	}
	for i := range t.Columns {
		c := &t.Columns[i]
		c.Title = clipRunes(c.Title, maxCell)
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
			r.Cells[j] = clipRunes(r.Cells[j], maxCell)
		}
	}
	return nil
}

func (p *Progress) check() error {
	if len(p.Bars) == 0 || len(p.Bars) > MaxBars {
		return fmt.Errorf("progress: needs 1 to %d bars, got %d", MaxBars, len(p.Bars))
	}
	for i := range p.Bars {
		b := &p.Bars[i]
		if b.Value < 0 || b.Value > 1 {
			return fmt.Errorf("progress: bar %d value %v is outside 0 to 1", i+1, b.Value)
		}
		if err := checkTone(fmt.Sprintf("progress bar %d", i+1), b.Tone); err != nil {
			return err
		}
		b.Label = clipRunes(b.Label, maxCell)
	}
	return nil
}

func (k *KeyValue) check() error {
	if len(k.Items) == 0 || len(k.Items) > MaxItems {
		return fmt.Errorf("keyvalue: needs 1 to %d items, got %d", MaxItems, len(k.Items))
	}
	for i := range k.Items {
		it := &k.Items[i]
		if err := checkTone(fmt.Sprintf("keyvalue item %d", i+1), it.Tone); err != nil {
			return err
		}
		it.Key, it.Value = clipRunes(it.Key, maxCell), clipRunes(it.Value, maxCell)
	}
	return nil
}

func (c *Chart) check() error {
	if c.Type != "bar" && c.Type != "line" {
		return fmt.Errorf("chart: type %q is not bar or line", c.Type)
	}
	if len(c.Series) == 0 || len(c.Series) > MaxSeries {
		return fmt.Errorf("chart: needs 1 to %d series, got %d", MaxSeries, len(c.Series))
	}
	if len(c.Labels) > MaxPoints {
		return fmt.Errorf("chart: %d labels; the limit is %d", len(c.Labels), MaxPoints)
	}
	n := len(c.Series[0].Values)
	for i := range c.Series {
		s := &c.Series[i]
		if len(s.Values) == 0 || len(s.Values) > MaxPoints {
			return fmt.Errorf("chart: series %d needs 1 to %d values, got %d", i+1, MaxPoints, len(s.Values))
		}
		if len(s.Values) != n {
			return fmt.Errorf("chart: every series needs the same number of values")
		}
		if err := checkTone(fmt.Sprintf("chart series %d", i+1), s.Tone); err != nil {
			return err
		}
		s.Name = clipRunes(s.Name, maxLabel)
	}
	if len(c.Labels) > 0 && len(c.Labels) != n {
		return fmt.Errorf("chart: %d labels for %d values", len(c.Labels), n)
	}
	for i := range c.Labels {
		c.Labels[i] = clipRunes(c.Labels[i], maxLabel)
	}
	c.Unit = clipRunes(c.Unit, maxLabel)
	return nil
}

func (b *Buttons) check() error {
	if len(b.Buttons) == 0 || len(b.Buttons) > MaxButtons {
		return fmt.Errorf("buttons: needs 1 to %d buttons, got %d", MaxButtons, len(b.Buttons))
	}
	seen := map[string]bool{}
	for i, x := range b.Buttons {
		l := strings.TrimSpace(x.Label)
		if l == "" || longer(l, maxLabel) {
			return fmt.Errorf("buttons: button %d label must be 1 to %d characters", i+1, maxLabel)
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
	b.Prompt = clipRunes(b.Prompt, maxCell)
	return nil
}

func (f *Form) check() error {
	if len(f.Fields) == 0 || len(f.Fields) > MaxFields {
		return fmt.Errorf("form: needs 1 to %d fields, got %d", MaxFields, len(f.Fields))
	}
	if longer(f.SubmitLabel, maxLabel) {
		return fmt.Errorf("form: submit_label is longer than %d characters", maxLabel)
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
		x.Label = clipRunes(x.Label, maxCell)
		x.Placeholder = clipRunes(x.Placeholder, maxCell)
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
			if len(x.Options) == 0 || len(x.Options) > MaxOptions {
				return fmt.Errorf("form: field %s needs 1 to %d options", x.Name, MaxOptions)
			}
			for j := range x.Options {
				x.Options[j] = clipRunes(x.Options[j], maxLabel)
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

func Event(w *Widget, ans Answer) string {
	b, _ := json.Marshal(ans)
	return fmt.Sprintf(`<switchboard-widget id="%s" kind="%s">%s</switchboard-widget>`, w.ID, w.Kind, b)
}

type Reply struct {
	ID     string
	Answer Answer
}

func Replies(text string) ([]Reply, string) {
	var out []Reply
	for _, m := range eventRE.FindAllStringSubmatch(text, -1) {
		var ans Answer
		if json.Unmarshal([]byte(m[3]), &ans) == nil {
			out = append(out, Reply{ID: m[1], Answer: ans})
		}
	}
	return out, strings.TrimSpace(eventRE.ReplaceAllString(text, ""))
}

func EventLabel(text string) string {
	rs, _ := Replies(text)
	if len(rs) == 0 {
		return text
	}
	if c := rs[0].Answer.Clicked; c != "" {
		return "Clicked “" + c + "”"
	}
	return "Submitted a form"
}

func ToolResult(text string) (id string, ok bool) {
	for _, verb := range []string{"shown ", "updated "} {
		if s, found := strings.CutPrefix(strings.TrimSpace(text), verb); found && idRE.MatchString(s) {
			return s, true
		}
	}
	return "", false
}
