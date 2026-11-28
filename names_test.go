package main

import (
	"testing"
	"time"

	caution "github.com/nullentropy/caution"
)

func TestALongToolNameIsShortenedToFitItsColumn(t *testing.T) {
	done := make(chan string)
	go func() { done <- toolName("mcp__claude_ai_Slack__slack_search_public_and_private") }()
	select {
	case got := <-done:
		if textW(got, 12, false) > 58 || got == "" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("toolName never returned")
	}
}

func TestPickingAFileReplacesOnlyTheMentionAtTheCaret(t *testing.T) {
	files := []string{"docs/towel.md", "web/src/Towel.tsx", "web/src/app.tsx", "README.md"}
	got := rankFiles(files, "tow", 8)
	if len(got) != 2 || got[0] != "docs/towel.md" {
		t.Fatalf("ranked %v", got)
	}
	value := "look at @Tow and fix it"
	caret := len([]rune("look at @Tow"))
	if out := insertMention(value, caret, "web/src/Towel.tsx"); out != "look at @web/src/Towel.tsx and fix it" {
		t.Fatalf("got %q", out)
	}
	if _, _, ok := mentionToken("mail arthur@example.example", -1); ok {
		t.Fatal("an email address is not a mention")
	}
}

func TestTheFirstSessionOnAnEmptyWallShowsUp(t *testing.T) {
	v := &View{wall: caution.Scroll(), empty: caution.Panel()}
	v.field = caution.Panel().Kids(v.wall)
	v.syncEmpty()
	if v.empty.Parent() != v.field || v.wall.Parent() != nil {
		t.Fatal("an empty wall does not show the empty state")
	}
	v.order = []*Agent{{}}
	v.syncEmpty()
	if v.wall.Parent() != v.field || v.empty.Parent() != nil {
		t.Fatal("the wall did not come back")
	}
}
