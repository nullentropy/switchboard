package main

import (
	"strings"
	"testing"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"
)

func monoText(spans []caution.Span) string {
	var b strings.Builder
	for _, s := range spans {
		if s.Mono {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func TestATableRendersAsAlignedMonospaceColumns(t *testing.T) {
	md := "Results:\n\n| Planet | Moons |\n|:------|------:|\n| Magrathea | 0 |\n| Earth | 1 |\n\nDone."
	spans := markdownSpans(md)
	want := "Planet     Moons\nMagrathea      0\nEarth          1\n"
	if got := monoText(spans); got != want {
		t.Errorf("table text = %q, want %q", got, want)
	}
	if got := plain(md); got != "Results:\n\n"+want+"\nDone." {
		t.Errorf("plain = %q", got)
	}
	for _, s := range spans {
		if strings.Contains(s.Text, "Planet") && !s.Bold {
			t.Errorf("header cell %q is not bold", s.Text)
		}
		if strings.Contains(s.Text, "Earth") && s.Bold {
			t.Errorf("body cell %q is bold", s.Text)
		}
	}
}

func TestATableCentresColumnsAndMeasuresInlineMarkupAsRendered(t *testing.T) {
	md := "| Name | Kind |\n|---|:---:|\n| `towel` | essential |\n| a \\| b | x |"
	want := "Name     Kind\ntowel  essential\na | b      x"
	if got := monoText(markdownSpans(md)); got != want {
		t.Errorf("table text = %q, want %q", got, want)
	}
}

func TestAPipeInProseIsNotATable(t *testing.T) {
	md := "Use a | b here\n---"
	if got := monoText(markdownSpans(md)); got != "" {
		t.Errorf("mono text = %q, want none", got)
	}
}

func TestAnIndentedBlockRendersAsCode(t *testing.T) {
	md := "Run this:\n\n    go test ./...\n\tgo vet ./...\n\n    echo 42\n\nThen check."
	want := "go test ./...\ngo vet ./...\n\necho 42\n"
	if got := monoText(markdownSpans(md)); got != want {
		t.Errorf("code text = %q, want %q", got, want)
	}
	if got := plain(md); got != "Run this:\n\n"+want+"\nThen check." {
		t.Errorf("plain = %q", got)
	}
}

func TestAnIndentedLineInsideAParagraphOrListStaysProse(t *testing.T) {
	for _, md := range []string{
		"First line\n    continues the paragraph",
		"- item\n\n    more about the item",
		"1. step\n\n    detail of the step",
	} {
		if got := monoText(markdownSpans(md)); got != "" {
			t.Errorf("%q: mono text = %q, want none", md, got)
		}
	}
}

func TestToolOutputExpandsTabsToEightColumnStops(t *testing.T) {
	got := trimLines("ok  \texample/drive\t0.042s\n1\tpackage drive", 14, 1600)
	want := "ok      example/drive   0.042s\n1       package drive"
	if got != want {
		t.Errorf("trimLines = %q, want %q", got, want)
	}
}

func TestChipWidthCountsATabToTheNextStop(t *testing.T) {
	if got, want := textW("ab\tc", 10, true), textW("ab      c", 10, true); got != want {
		t.Errorf("textW with a tab = %v, want %v", got, want)
	}
}

func TestErrorEntriesRenderInMonospace(t *testing.T) {
	v := openView(&Agent{}, &Agent{})
	v.focus.list = caution.VStack()
	sh := v.focus.addEntry(Entry{Kind: EntryError, Text: "claude exited: exit status 1"})
	rt := findType(sh.body, wire.RichText)
	if rt == nil {
		t.Fatal("the error entry has no rich text")
	}
	for _, s := range rt.Prop("spans").([]caution.Span) {
		if !s.Mono {
			t.Errorf("error span %q is not monospace", s.Text)
		}
	}
}
