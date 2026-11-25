package main

import (
	"regexp"
	"strings"
	"unicode/utf8"

	caution "github.com/nullentropy/caution"
)

var (
	inlineRE  = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|\\[[^\\]]+\\]\\([^)]+\\)")
	bulletRE  = regexp.MustCompile(`^(\s*)[-*+] `)
	orderedRE = regexp.MustCompile(`^\s*\d+[.)] `)
	ruleRE    = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
)

func markdownSpans(md string) []caution.Span {
	var out []caution.Span
	lines := strings.Split(strings.TrimRight(md, "\n"), "\n")
	inFence, inCode, inList := false, false, false
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		nl := "\n"
		if i == len(lines)-1 {
			nl = ""
		}
		if !inFence {
			if inCode && !codeLine(lines, i) {
				inCode = false
			}
			if !inCode && !inList && indented(ln) && (i == 0 || blank(lines[i-1])) {
				inCode = true
			}
			if inCode {
				out = append(out, caution.Span{Text: unindent(ln) + nl, Mono: true, Color: "$inkDim"})
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, caution.Span{Text: ln + nl, Mono: true, Color: "$inkDim"})
			continue
		}
		if !blank(ln) && !indented(ln) {
			inList = bulletRE.MatchString(ln) || orderedRE.MatchString(ln)
		}
		if isTable(lines, i) {
			end := i + 2
			for end < len(lines) && strings.Contains(lines[end], "|") && !blank(lines[end]) {
				end++
			}
			out = append(out, table(lines[i], lines[i+1], lines[i+2:end])...)
			if end < len(lines) {
				out = append(out, caution.Span{Text: "\n", Mono: true})
			}
			i = end - 1
			continue
		}
		if h := strings.TrimLeft(ln, "#"); len(h) < len(ln) && strings.HasPrefix(h, " ") {
			out = append(out, caution.Span{Text: strings.TrimSpace(h) + nl, Bold: true})
			continue
		}
		if m := bulletRE.FindStringSubmatch(ln); m != nil {
			out = append(out, caution.Span{Text: m[1] + "• ", Color: "$inkFaint"})
			ln = ln[len(m[0]):]
		}
		out = append(out, inline(ln+nl)...)
	}
	return out
}

func blank(ln string) bool { return strings.TrimSpace(ln) == "" }

func indented(ln string) bool {
	return !blank(ln) && (strings.HasPrefix(ln, "\t") || strings.HasPrefix(ln, "    "))
}

func unindent(ln string) string {
	if strings.HasPrefix(ln, "\t") {
		return ln[1:]
	}
	return strings.TrimPrefix(ln, "    ")
}

func codeLine(lines []string, i int) bool {
	if indented(lines[i]) {
		return true
	}
	if !blank(lines[i]) {
		return false
	}
	for _, ln := range lines[i+1:] {
		if !blank(ln) {
			return indented(ln)
		}
	}
	return false
}

func isTable(lines []string, i int) bool {
	return i+1 < len(lines) && strings.Contains(lines[i], "|") &&
		strings.Contains(lines[i+1], "|") && strings.Contains(lines[i+1], "-") && ruleRE.MatchString(lines[i+1])
}

func cells(ln string) []string {
	ln = strings.TrimSpace(ln)
	ln = strings.TrimPrefix(ln, "|")
	if strings.HasSuffix(ln, "|") && !strings.HasSuffix(ln, `\|`) {
		ln = ln[:len(ln)-1]
	}
	var out []string
	var b strings.Builder
	for i := 0; i < len(ln); i++ {
		switch {
		case ln[i] == '\\' && i+1 < len(ln) && ln[i+1] == '|':
			b.WriteByte('|')
			i++
		case ln[i] == '|':
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(ln[i])
		}
	}
	return append(out, strings.TrimSpace(b.String()))
}

func table(head, rule string, body []string) []caution.Span {
	aligns := cells(rule)
	n := len(cells(head))
	var rows [][][]caution.Span
	for _, ln := range append([]string{head}, body...) {
		cs := cells(ln)
		row := make([][]caution.Span, n)
		for c := range row {
			if c < len(cs) {
				row[c] = inline(cs[c])
			}
		}
		rows = append(rows, row)
	}
	widths := make([]int, n)
	for _, row := range rows {
		for c, cell := range row {
			widths[c] = max(widths[c], spanLen(cell))
		}
	}
	var out []caution.Span
	pad := func(k int) {
		if k > 0 {
			out = append(out, caution.Span{Text: strings.Repeat(" ", k), Mono: true})
		}
	}
	for r, row := range rows {
		if r > 0 {
			out = append(out, caution.Span{Text: "\n", Mono: true})
		}
		for c, cell := range row {
			if c > 0 {
				pad(2)
			}
			free := widths[c] - spanLen(cell)
			left := 0
			if c < len(aligns) {
				a := aligns[c]
				switch {
				case strings.HasPrefix(a, ":") && strings.HasSuffix(a, ":"):
					left = free / 2
				case strings.HasSuffix(a, ":"):
					left = free
				}
			}
			pad(left)
			for _, s := range cell {
				s.Mono = true
				s.Bold = s.Bold || r == 0
				out = append(out, s)
			}
			if c < n-1 {
				pad(free - left)
			}
		}
	}
	return out
}

func spanLen(spans []caution.Span) int {
	n := 0
	for _, s := range spans {
		n += utf8.RuneCountInString(s.Text)
	}
	return n
}

func inline(s string) []caution.Span {
	var out []caution.Span
	last := 0
	for _, loc := range inlineRE.FindAllStringIndex(s, -1) {
		if loc[0] > last {
			out = append(out, caution.Span{Text: s[last:loc[0]]})
		}
		tok := s[loc[0]:loc[1]]
		switch {
		case strings.HasPrefix(tok, "`"):
			out = append(out, caution.Span{Text: tok[1 : len(tok)-1], Mono: true, Bg: "$codeBg"})
		case strings.HasPrefix(tok, "**"):
			out = append(out, caution.Span{Text: tok[2 : len(tok)-2], Bold: true})
		default:
			text := tok[1:strings.Index(tok, "](")]
			out = append(out, caution.Span{Text: text, Color: "$accent"})
		}
		last = loc[1]
	}
	if last < len(s) {
		out = append(out, caution.Span{Text: s[last:]})
	}
	return out
}

func plain(md string) string {
	var b strings.Builder
	for _, s := range markdownSpans(md) {
		b.WriteString(s.Text)
	}
	return b.String()
}
