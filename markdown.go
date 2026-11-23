package main

import (
	"regexp"
	"strings"

	caution "github.com/nullentropy/caution"
)

var (
	inlineRE = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|\\[[^\\]]+\\]\\([^)]+\\)")
	bulletRE = regexp.MustCompile(`^(\s*)[-*+] `)
)

func markdownSpans(md string) []caution.Span {
	var out []caution.Span
	lines := strings.Split(strings.TrimRight(md, "\n"), "\n")
	inFence := false
	for i, ln := range lines {
		nl := "\n"
		if i == len(lines)-1 {
			nl = ""
		}
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, caution.Span{Text: ln + nl, Mono: true, Color: "$inkDim"})
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
