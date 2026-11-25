package main

import (
	"testing"

	caution "github.com/nullentropy/caution"
)

func openView(open *Agent, d *Agent) *View {
	v := &View{hub: &Hub{Dispatcher: d}, cmd: caution.TextField(""), body: caution.Panel(),
		header: caution.Panel(), statsRow: caution.Panel(), compact: caution.Panel()}
	v.shell = caution.Panel()
	v.body.Add(v.shell)
	v.header.Add(v.compact)
	v.focus = &focusView{v: v, a: open, node: caution.Panel(), composer: caution.Textarea("")}
	v.shell.Add(v.focus.node)
	v.showDialog(caution.Panel())
	return v
}

func TestAskTheDispatcherInItsConversationKeepsItOpen(t *testing.T) {
	d := &Agent{}
	v := openView(d, d)
	v.focusCommand()
	if v.dialog != nil {
		t.Fatal("the dialog on top is still open")
	}
	if v.focus == nil || v.focus.a != d || v.shell.Parent() == nil {
		t.Fatal("the dispatcher's conversation was closed")
	}
}

func TestAskTheDispatcherInAnotherSessionGoesBackToTheBoard(t *testing.T) {
	v := openView(&Agent{}, &Agent{})
	v.focusCommand()
	if v.dialog != nil || v.focus != nil || v.shell.Parent() != nil {
		t.Fatal("the session and its dialog should both close")
	}
}

func TestCmdLInASessionStaysOnIt(t *testing.T) {
	a := &Agent{}
	v := openView(a, &Agent{})
	v.focusInput()
	if v.dialog != nil {
		t.Fatal("the dialog on top is still open")
	}
	if v.focus == nil || v.focus.a != a || v.shell.Parent() == nil {
		t.Fatal("the session was closed")
	}
}

func TestCmdLOnTheBoardStaysOnTheBoard(t *testing.T) {
	v := openView(&Agent{}, &Agent{})
	v.closeFocus()
	v.focusInput()
	if v.dialog != nil || v.focus != nil || v.shell.Parent() != nil {
		t.Fatal("the board should show with no dialog")
	}
}
