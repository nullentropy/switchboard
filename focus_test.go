package main

import (
	"testing"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/wire"
)

const scrollbarW = 10.0

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
	inset := anchorAt(t, list, "right")
	if inset < scrollbarW {
		t.Errorf("the answer choices end %vpx from the scroll view's right edge and run under the %vpx scrollbar", inset, scrollbarW)
	}
	if margin := anchorAt(t, scroll, "right") + inset; margin != 20 {
		t.Errorf("the answer choices end %vpx from the dialog's right edge, want 20", margin)
	}
}
