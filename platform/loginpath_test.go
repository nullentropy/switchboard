package platform

import (
	"os"
	"testing"
)

func TestAnAppOpenedFromFinderFindsAgentsOnTheLoginShellsPath(t *testing.T) {
	dir := t.TempDir()
	shell := dir + "/fakesh"
	os.WriteFile(shell, []byte("#!/bin/sh\necho 'Share and enjoy'\nPATH=/Users/arthur/.local/bin:/usr/bin\neval \"$2\"\n"), 0o755)
	if got := loginPath(shell); got != "/Users/arthur/.local/bin:/usr/bin" {
		t.Fatalf("got %q", got)
	}
	if got := loginPath(dir + "/missing"); got != "" {
		t.Fatalf("a broken shell gave %q", got)
	}
}
