package main

import (
	"regexp"
	"strings"

	caution "github.com/nullentropy/caution"
)

var linkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func (f *focusView) copyMenus(row *caution.Node, md string, spans []caution.Span) *caution.Node {
	write := func(s string) { f.v.s.WriteClipboard(s) }
	text := spanText(spans)
	clamp := func(start, end int) (int, int) {
		start, end = max(0, min(start, len(text))), max(0, min(end, len(text)))
		return min(start, end), max(start, end)
	}
	return row.
		Context(caution.ContextItem{Title: "Copy message", OnPick: func() { write(md) }}).
		SelectionContext(
			caution.SelectionItem{Title: "Copy", OnPick: func(start, end int) {
				start, end = clamp(start, end)
				write(text[start:end])
			}},
			caution.SelectionItem{Title: "Copy as Markdown", OnPick: func(start, end int) {
				start, end = clamp(start, end)
				write(selectionMarkdown(spans, md, start, end))
			}},
			caution.SelectionItem{Title: "Copy whole message", OnPick: func(_, _ int) { write(md) }},
		)
}

func spanText(spans []caution.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

type mdPiece struct {
	caution.Span
	url string
}

type mdLine struct {
	pieces []mdPiece
	monoNL bool
}

func (l mdLine) code() bool {
	if len(l.pieces) == 0 {
		return l.monoNL
	}
	block := false
	for _, p := range l.pieces {
		if !p.Mono {
			return false
		}
		block = block || p.Bg != "$codeBg"
	}
	return block
}

func selectionMarkdown(spans []caution.Span, md string, start, end int) string {
	links := linkRE.FindAllStringSubmatch(md, -1)
	lines := []mdLine{{}}
	off := 0
	for _, sp := range spans {
		base := off
		off += len(sp.Text)
		url := ""
		if sp.Color == "$accent" && !sp.Mono {
			for i, l := range links {
				if l[1] == sp.Text {
					url, links = l[2], links[i+1:]
					break
				}
			}
		}
		a, b := max(start, base), min(end, off)
		if a >= b {
			continue
		}
		for i, part := range strings.Split(sp.Text[a-base:b-base], "\n") {
			if i > 0 {
				lines[len(lines)-1].monoNL = sp.Mono
				lines = append(lines, mdLine{})
			}
			if part != "" {
				p := mdPiece{Span: sp, url: url}
				p.Text = part
				lines[len(lines)-1].pieces = append(lines[len(lines)-1].pieces, p)
			}
		}
	}
	var out []string
	fenced := false
	for _, ln := range lines {
		code := ln.code()
		if code != fenced {
			out = append(out, "```")
			fenced = code
		}
		var b strings.Builder
		for _, p := range ln.pieces {
			if code {
				b.WriteString(p.Text)
			} else {
				b.WriteString(styled(p))
			}
		}
		out = append(out, b.String())
	}
	if fenced {
		out = append(out, "```")
	}
	return strings.Join(out, "\n")
}

func styled(p mdPiece) string {
	t := p.Text
	if p.Color == "$inkFaint" {
		return strings.Replace(t, "•", "-", 1)
	}
	core := strings.TrimSpace(t)
	if core == "" {
		return t
	}
	lead := t[:strings.Index(t, core)]
	trail := t[len(lead)+len(core):]
	switch {
	case p.url != "":
		core = "[" + core + "](" + p.url + ")"
	case p.Mono:
		core = "`" + core + "`"
	}
	if p.Italic {
		core = "*" + core + "*"
	}
	if p.Bold {
		core = "**" + core + "**"
	}
	return lead + core + trail
}
