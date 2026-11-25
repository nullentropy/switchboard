package main

import (
	"slices"
	"strings"
	"testing"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"
)

const copyMD = "# Towels\n\nAlways know **where** yours is, see `towel.go` and [the guide](https://hhgttg.example).\n\n- dry\n- dent\n\n```\nfor {\n\n}\n```\n\nDone."

func TestCopyAsMarkdownRebuildsTheWholeMessage(t *testing.T) {
	spans := markdownSpans(copyMD)
	got := selectionMarkdown(spans, copyMD, 0, len(spanText(spans)))
	want := "**Towels**\n\nAlways know **where** yours is, see `towel.go` and [the guide](https://hhgttg.example).\n\n- dry\n- dent\n\n```\nfor {\n\n}\n```\n\nDone."
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCopyAsMarkdownKeepsStylesInAPartialSelection(t *testing.T) {
	spans := markdownSpans(copyMD)
	text := spanText(spans)
	from := strings.Index(text, "here")
	to := strings.Index(text, "guide") + len("guide")
	got := selectionMarkdown(spans, copyMD, from, to)
	want := "**here** yours is, see `towel.go` and [the guide](https://hhgttg.example)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	from = strings.Index(text, "dent")
	to = strings.Index(text, "}") + 1
	got = selectionMarkdown(spans, copyMD, from, to)
	want = "dent\n\n```\nfor {\n\n}\n```"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func menuTitles(n *caution.Node, prop string) []string {
	specs, _ := n.Prop(prop).([]any)
	var out []string
	for _, s := range specs {
		out = append(out, s.(map[string]any)["title"].(string))
	}
	return out
}

func TestTranscriptMessagesCarryTheCopyMenus(t *testing.T) {
	v := openView(&Agent{}, &Agent{})
	f := v.focus
	f.list = caution.VStack()
	for _, e := range []Entry{
		{Kind: EntryUser, Text: "hi"},
		{Kind: EntryAssistant, Text: "**hello**"},
		{Kind: EntryError, Text: "boom"},
	} {
		f.addEntry(e)
		kids := f.list.Children()
		row := kids[len(kids)-1]
		if got := menuTitles(row, "context"); !slices.Equal(got, []string{"Copy message"}) {
			t.Errorf("%v row menu %v", e.Kind, got)
		}
		if got := menuTitles(row, "selectionContext"); !slices.Equal(got, []string{"Copy", "Copy as Markdown", "Copy whole message"}) {
			t.Errorf("%v row selection menu %v", e.Kind, got)
		}
		if findType(row, wire.RichText) == nil {
			t.Errorf("%v row has no text", e.Kind)
		}
	}
}
