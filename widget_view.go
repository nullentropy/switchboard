package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	caution "github.com/nullentropy/caution"
)

const widgetRedraw = 250 * time.Millisecond

func toneColor(t, fallback string) string {
	switch t {
	case "good":
		return "$stReady"
	case "warn":
		return "$stNeed"
	case "bad":
		return "$stFail"
	}
	return fallback
}

func wtext(s string, size float64, color string, mono bool) *caution.Node {
	return caution.RichText(caution.Span{Text: s, Mono: mono}).FontSize(size).Color(color).Selectable()
}

func (f *focusView) addWidget(e Entry) shown {
	gutter := caution.Label(f.name()).FontSize(12).Weight(600).W(72).Color("$ink")
	slot := caution.VStack().Fill(1).Align("stretch")
	f.list.Add(caution.HStack().Gap(12).Kids(gutter, slot))
	sh := shown{rev: e.Rev, gutter: gutter, body: slot, idx: len(f.shown), wid: e.Widget.ID}
	f.drawWidget(&sh, e)
	return sh
}

func (f *focusView) refreshWidget(sh *shown, e Entry) {
	wait := time.Until(sh.drawn.Add(widgetRedraw))
	if wait <= 0 {
		f.drawWidget(sh, e)
		return
	}
	if sh.due {
		return
	}
	sh.due = true
	idx, wid := sh.idx, sh.wid
	time.AfterFunc(wait, func() {
		f.v.s.Update(func() {
			if idx >= len(f.shown) || f.shown[idx].wid != wid || !f.shown[idx].due {
				return
			}
			es := f.a.Snapshot().Entries
			if idx < len(es) && es[idx].Kind == EntryWidget {
				f.drawWidget(&f.shown[idx], es[idx])
			}
		})
	})
}

func (f *focusView) drawWidget(sh *shown, e Entry) {
	sh.due = false
	sh.drawn = time.Now()
	sh.rev = e.Rev
	sh.wid = e.Widget.ID
	sh.body.Clear()
	a := f.a
	sh.body.Add(widgetCard(e, func(ans WidgetAnswer) { go a.AnswerWidget(e.Widget.ID, ans) }))
}

func widgetCard(e Entry, answer func(WidgetAnswer)) *caution.Node {
	w := e.Widget
	card := caution.VStack().Gap(10).Pad(14).Align("stretch").Border("$edge", 1).Radius(8)
	if w.Title != "" {
		card.Add(caution.RichText(caution.Span{Text: w.Title}).FontSize(13).Weight(600).Color(toneColor(w.Tone, "$ink")).Selectable())
	}
	switch w.Kind {
	case "table":
		card.Add(tableWidget(w.Table))
	case "progress":
		card.Add(progressWidget(w.Progress))
	case "keyvalue":
		card.Add(keyValueWidget(w.KeyValue))
	case "chart":
		card.Add(chartWidget(w.Chart))
	case "buttons":
		card.Add(buttonsWidget(w.Buttons, e.Answer, answer))
	case "form":
		card.Add(formWidget(w.Form, e.Answer, answer))
	}
	return card
}

func tableWidget(t *WTable) *caution.Node {
	cols := make([]caution.GridCol, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = caution.GridCol{Weight: 1, Align: map[bool]string{true: "end", false: "start"}[c.Align == "right"]}
	}
	g := caution.Grid(cols).ColGap(16).RowGap(6)
	for _, c := range t.Columns {
		g.Add(wtext(c.Title, 12, "$inkFaint", false).Weight(600))
	}
	for _, r := range t.Rows {
		for j, cell := range r.Cells {
			g.Add(wtext(cell, 13, toneColor(r.Tone, "$ink"), t.Columns[j].Mono))
		}
	}
	if len(t.Rows) == 0 {
		return caution.VStack().Gap(6).Align("stretch").Kids(g, wtext("No rows", 12, "$inkFaint", false))
	}
	if len(t.Rows) <= 15 {
		return g
	}
	return caution.Scroll().H(400).Kids(
		caution.VStack().Align("stretch").Anchor(caution.A{Left: px(0), Right: px(12), Top: px(0)}).Kids(g))
}

func meter(frac float64, color string, h float64) *caution.Node {
	frac = math.Max(0, math.Min(1, frac))
	return caution.HStack().H(h).Radius(h/2).Bg("$panelInset").Clips().Kids(
		caution.Panel().Fill(math.Max(frac, 1e-4)).H(h).Bg(color),
		caution.Panel().Fill(math.Max(1-frac, 1e-4)).H(h))
}

func progressWidget(p *WProgress) *caution.Node {
	col := caution.VStack().Gap(10).Align("stretch")
	for _, b := range p.Bars {
		col.Add(caution.VStack().Gap(4).Align("stretch").Kids(
			caution.HStack().Gap(8).Kids(
				wtext(b.Label, 13, "$ink", false).Fill(1),
				wtext(strconv.Itoa(int(math.Round(b.Value*100)))+"%", 12, "$inkDim", true)),
			meter(b.Value, toneColor(b.Tone, "$accent"), 6)))
	}
	return col
}

func keyValueWidget(k *WKeyValue) *caution.Node {
	g := caution.Grid([]caution.GridCol{{}, {Weight: 1}}).ColGap(16).RowGap(6)
	for _, it := range k.Items {
		g.Add(wtext(it.Key, 13, "$inkDim", false))
		g.Add(wtext(it.Value, 13, toneColor(it.Tone, "$ink"), false))
	}
	return g
}

func seriesColor(s WSeries, i int) string {
	return toneColor(s.Tone, []string{"$accent", "$phRead"}[i%2])
}

func fmtNum(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if a := math.Abs(v); a >= 1000 || (a > 0 && a < 0.01) || len(s) > 9 {
		s = strconv.FormatFloat(v, 'g', 4, 64)
	}
	return s + unit
}

func chartWidget(c *WChart) *caution.Node {
	col := caution.VStack().Gap(10).Align("stretch")
	if len(c.Series) > 1 || c.Series[0].Name != "" {
		legend := caution.HStack().Gap(14).Align("center")
		for i, s := range c.Series {
			legend.Add(caution.HStack().Gap(6).Align("center").Kids(
				caution.Panel().W(10).H(10).Radius(2).Bg(seriesColor(s, i)),
				wtext(s.Name, 12, "$inkDim", false)))
		}
		col.Add(legend)
	}
	lo, hi := 0.0, 0.0
	for _, s := range c.Series {
		for _, v := range s.Values {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	if c.Type == "bar" {
		g := caution.Grid([]caution.GridCol{{}, {Weight: 1}, {Align: "end"}}).ColGap(10).RowGap(6)
		for i := range c.Series[0].Values {
			label := ""
			if i < len(c.Labels) {
				label = c.Labels[i]
			}
			bars := caution.VStack().Gap(2).Align("stretch")
			var vals []string
			for si, s := range c.Series {
				bars.Add(meter((s.Values[i]-lo)/span, seriesColor(s, si), 10))
				vals = append(vals, fmtNum(s.Values[i], c.Unit))
			}
			g.Add(wtext(label, 12, "$inkDim", false))
			g.Add(bars)
			g.Add(wtext(strings.Join(vals, " / "), 12, "$inkDim", true))
		}
		col.Add(g)
		return col
	}
	const plotH = 120.0
	plot := caution.HStack().Gap(1).H(plotH).Align("stretch")
	for i := range c.Series[0].Values {
		cell := caution.Panel().Fill(1).H(plotH)
		for si, s := range c.Series {
			y := (s.Values[i] - lo) / span * (plotH - 4)
			cell.Add(caution.Panel().H(4).Radius(2).Bg(seriesColor(s, si)).
				Anchor(caution.A{Left: px(0), Right: px(0), Bottom: px(y)}))
		}
		if i < len(c.Labels) {
			var vals []string
			for _, s := range c.Series {
				vals = append(vals, fmtNum(s.Values[i], c.Unit))
			}
			cell.Tip(c.Labels[i] + ": " + strings.Join(vals, " / "))
		}
		plot.Add(cell)
	}
	axis := caution.VStack().W(64).H(plotH).Kids(
		wtext(fmtNum(hi, c.Unit), 11, "$inkFaint", true), caution.Panel().Fill(1), wtext(fmtNum(lo, c.Unit), 11, "$inkFaint", true))
	col.Add(caution.HStack().Gap(8).Align("stretch").Kids(axis,
		caution.Panel().Fill(1).H(plotH).Bg("$panelInset").Radius(4).Clips().Kids(plot.Anchor(fill()))))
	if n := len(c.Labels); n > 0 {
		col.Add(caution.HStack().Kids(caution.Panel().W(72),
			wtext(c.Labels[0], 11, "$inkFaint", false), caution.Panel().Fill(1), wtext(c.Labels[n-1], 11, "$inkFaint", false)))
	}
	return col
}

func buttonsWidget(b *WButtons, ans *WidgetAnswer, answer func(WidgetAnswer)) *caution.Node {
	col := caution.VStack().Gap(10).Align("stretch")
	if b.Prompt != "" {
		col.Add(wtext(b.Prompt, 13, "$ink", false))
	}
	if ans != nil {
		col.Add(wtext("You chose “"+ans.Clicked+"”", 13, "$inkDim", false))
		return col
	}
	row := caution.HStack().Gap(8).Align("center")
	sent := false
	for _, x := range b.Buttons {
		label := x.Label
		btn := caution.Button(label).OnClick(func() {
			if !sent {
				sent = true
				answer(WidgetAnswer{Clicked: label})
			}
		})
		if c := toneColor(x.Tone, ""); c != "" {
			btn.Color(c)
		}
		row.Add(btn)
	}
	return col.Add(row)
}

func formWidget(fm *WForm, ans *WidgetAnswer, answer func(WidgetAnswer)) *caution.Node {
	g := caution.Grid([]caution.GridCol{{}, {Weight: 1}}).ColGap(14).RowGap(8)
	if ans != nil {
		for _, x := range fm.Fields {
			g.Add(wtext(x.Label, 13, "$inkDim", false))
			v := ans.Values[x.Name]
			if v == "" {
				g.Add(wtext("(empty)", 13, "$inkFaint", false))
			} else {
				g.Add(wtext(v, 13, "$ink", false))
			}
		}
		return caution.VStack().Gap(8).Align("stretch").Kids(g, wtext("Submitted", 12, "$inkFaint", false))
	}
	vals := map[string]string{}
	errL := caution.Label("").FontSize(12).Color("$stFail")
	sent := false
	submit := func() {
		if sent {
			return
		}
		for _, x := range fm.Fields {
			if x.Required && strings.TrimSpace(vals[x.Name]) == "" {
				errL.SetText(fmt.Sprintf("Fill in %s.", x.Label))
				return
			}
		}
		sent = true
		out := map[string]string{}
		for _, x := range fm.Fields {
			out[x.Name] = vals[x.Name]
		}
		answer(WidgetAnswer{Values: out})
	}
	for _, x := range fm.Fields {
		name := x.Name
		vals[name] = x.Default
		label := x.Label
		if x.Required {
			label += " *"
		}
		g.Add(wtext(label, 13, "$inkDim", false))
		switch x.Kind {
		case "text":
			fld := caution.TextField(x.Default).Placeholder(x.Placeholder)
			fld.OnInput(func(s string) { vals[name] = s })
			fld.OnSubmit(func(s string) { vals[name] = s; submit() })
			g.Add(fld)
		case "textarea":
			fld := caution.Textarea(x.Default).Placeholder(x.Placeholder).Rows(x.Rows)
			fld.OnInput(func(s string) { vals[name] = s })
			g.Add(fld)
		case "choice":
			sel := 0
			for i, o := range x.Options {
				if o == x.Default {
					sel = i
				}
			}
			opts := x.Options
			vals[name] = opts[sel]
			pick := func(i int) {
				if i >= 0 && i < len(opts) {
					vals[name] = opts[i]
				}
			}
			if len(opts) <= 4 {
				g.Add(caution.Radio(opts, sel).OnSelect(pick))
			} else {
				g.Add(caution.Select(opts, sel).OnSelect(pick))
			}
		}
	}
	label := fm.SubmitLabel
	if label == "" {
		label = "Submit"
	}
	return caution.VStack().Gap(10).Align("stretch").Kids(g,
		caution.HStack().Gap(10).Align("center").Kids(caution.Button(label).OnClick(submit), errL))
}
