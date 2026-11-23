package main

import (
	"strings"
	"testing"

	caution "github.com/nullentropy/caution"
)

func TestJAndKInTheSessionListReachTheDispatcherRow(t *testing.T) {
	d, one, two := &Agent{}, &Agent{}, &Agent{}
	name := map[*Agent]string{d: "dispatcher", one: "Ford", two: "Zaphod"}
	sessions := []*Agent{one, two}
	steps := []struct {
		from *Agent
		dir  int
		want *Agent
	}{
		{one, -1, d},
		{d, 1, one},
		{d, -1, two},
		{two, 1, d},
		{one, 1, two},
	}
	for _, st := range steps {
		if got := railStep(d, sessions, st.from, st.dir); got != st.want {
			t.Errorf("from %s by %d: got %s, want %s", name[st.from], st.dir, name[got], name[st.want])
		}
	}
	if got := railStep(d, sessions, &Agent{}, 1); got != nil {
		t.Errorf("from a session not in the list: got %s, want nothing", name[got])
	}
}

func railView(d *Agent, sessions ...*Agent) *View {
	v := &View{hub: &Hub{Dispatcher: d}, tiles: map[*Agent]*tile{}, body: caution.Panel()}
	v.buildBoard()
	for _, a := range sessions {
		v.addTile(a)
	}
	v.buildShell()
	v.focus = &focusView{v: v, a: sessions[0]}
	return v
}

func railTexts(n *caution.Node) []string {
	var out []string
	for _, k := range n.Children() {
		if s, ok := k.Prop("text").(string); ok && s != "" {
			out = append(out, s)
		}
		if s, ok := k.Prop("tip").(string); ok && s != "" {
			out = append(out, s)
		}
		if s, ok := k.Prop("label").(string); ok && s != "" {
			out = append(out, s)
		}
		out = append(out, railTexts(k)...)
	}
	return out
}

func TestTheSessionListKeepsTheDispatcherRowAndNamesTheFilterThatHidesSessions(t *testing.T) {
	d := &Agent{Meta: Meta{Title: "Dispatcher"}}
	arthur := &Agent{Meta: Meta{Title: "Arthur Dent", Cwd: "/Users/ford/heart-of-gold", Goal: "Find the Question"}}
	trillian := &Agent{Meta: Meta{Title: "Trillian", Cwd: "/Users/ford/code"}}
	v := railView(d, arthur, trillian)
	v.goal = "Find the Question"
	v.layoutRail()

	kids := v.railList.Children()
	if len(kids) == 0 || kids[0] != v.rows[d].node {
		t.Fatal("the Dispatcher row is not first in the list while a goal filter is on")
	}
	if v.rows[trillian] != nil && v.rows[trillian].node.Parent() != nil {
		t.Fatal("Trillian is outside the goal and should be hidden")
	}
	texts := strings.Join(railTexts(v.railList), "|")
	if !strings.Contains(texts, "Find the Question") || !strings.Contains(texts, "Show all") {
		t.Fatalf("the list does not name the goal filter or offer to clear it: %s", texts)
	}

	v.goal, v.project = "", "heart-of-gold"
	v.layoutRail()
	texts = strings.Join(railTexts(v.railList), "|")
	if !strings.Contains(texts, "heart-of-gold") || v.railList.Children()[0] != v.rows[d].node {
		t.Fatalf("the project filter is not shown under the Dispatcher row: %s", texts)
	}
}

func TestShowAllInTheSessionListClearsTheFilter(t *testing.T) {
	d := &Agent{Meta: Meta{Title: "Dispatcher"}}
	arthur := &Agent{Meta: Meta{Title: "Arthur Dent", Cwd: "/Users/ford/heart-of-gold", Goal: "Find the Question"}}
	trillian := &Agent{Meta: Meta{Title: "Trillian", Cwd: "/Users/ford/code"}}
	v := railView(d, arthur, trillian)
	v.goal, v.project = "Find the Question", "heart-of-gold"
	v.clearFilter()
	if v.goal != "" || v.project != "" {
		t.Fatalf("the filter is still on: goal %q, project %q", v.goal, v.project)
	}
	if r := v.rows[trillian]; r == nil || r.node.Parent() != v.railList {
		t.Fatal("Trillian is still hidden after Show all")
	}
	if strings.Contains(strings.Join(railTexts(v.railList), "|"), "Show all") {
		t.Fatal("the filter bar is still shown with no filter on")
	}
}
