package main

import (
	"path/filepath"
	"testing"
)

func TestLinuxWorktreesLiveInTheXDGDataDirectory(t *testing.T) {
	t.Setenv("HOME", "/home/arthur")
	t.Setenv("XDG_DATA_HOME", "")
	if got := worktreeHome(); got != filepath.Join("/home/arthur", ".local", "share", "switchboard", "worktrees") {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "/srv/heart-of-gold")
	if got := worktreeHome(); got != "/srv/heart-of-gold/switchboard/worktrees" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "relative/magrathea")
	if got := worktreeHome(); got != filepath.Join("/home/arthur", ".local", "share", "switchboard", "worktrees") {
		t.Fatalf("a relative XDG_DATA_HOME gave %q", got)
	}
}
