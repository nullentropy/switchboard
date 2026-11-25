package main

import (
	"testing"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"
)

const (
	scrollThumbW     = 6.0
	scrollThumbInset = 4.0
)

func findType(n *caution.Node, typ wire.Type) *caution.Node {
	if n.Type() == typ {
		return n
	}
	for _, k := range n.Children() {
		if hit := findType(k, typ); hit != nil {
			return hit
		}
	}
	return nil
}

func TestTheQuestionDialogKeepsAnswerChoicesClearOfTheScrollbar(t *testing.T) {
	v := openView(&Agent{}, &Agent{})
	var opts []Option
	for range 30 {
		opts = append(opts, Option{Label: "Towel", Description: "Always know where yours is"})
	}
	v.focus.ask(&Pending{ReqID: "q", Tool: "AskUserQuestion", Questions: []Question{{Question: "Which one?", Options: opts}}})
	scroll := findType(v.dialog, wire.Scroll)
	if scroll == nil {
		t.Fatal("the question dialog has no scroll view")
	}
	list := scroll.Children()[0]
	padding := anchorAt(t, scroll, "left") + anchorAt(t, list, "left")
	gap := anchorAt(t, list, "right") - scrollThumbInset - scrollThumbW
	if gap != padding {
		t.Errorf("the answer choices end %vpx from the scrollbar, want %vpx to match the dialog's left padding", gap, padding)
	}
}
