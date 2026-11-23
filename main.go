package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	caution "github.com/nullentropy/caution"
	"github.com/nullentropy/caution/native"
)

func main() {
	addr := flag.String("addr", "", `serve browsers on this address, such as ":8790", instead of opening a window`)
	claudeBin := flag.String("claude", "", "path to the claude binary (default: from PATH)")
	codexBin := flag.String("codex", "", "path to the codex binary (default: from PATH)")
	geminiBin := flag.String("gemini", "", "path to the gemini binary (default: from PATH)")
	statePath := flag.String("state", defaultStatePath(), "where the session list and settings are kept")
	demo := flag.Bool("demo", false, "fill the window with pretend sessions, to look at the UI; nothing is saved")
	demoN := flag.Int("demo-sessions", 7, "how many pretend sessions -demo starts")
	flag.Parse()
	if native.InBundle() {
		logToFile()
		usePathFromLoginShell()
	}

	bins := map[string]string{"claude": *claudeBin, "codex": *codexBin, "gemini": *geminiBin}
	if *demo {
		*statePath = ""
		bins = map[string]string{"claude": "true", "codex": "true", "gemini": "true"}
	}
	hub := newHub(bins, *statePath)
	go hub.loadCodexModels()
	go hub.loadClaudeModels()
	if err := checkBin(hub, "claude"); err != nil {
		log.Fatal("claude is not on PATH; pass -claude /path/to/claude")
	}
	if *demo {
		hub.runDemo(*demoN)
	} else {
		go hub.watchLoad()
	}
	mcpURL, err := hub.startMCP()
	if err != nil {
		log.Fatalf("start the dispatcher's tool server: %v", err)
	}
	hub.Dispatcher.extraArgs = func() []string { return dispatcherArgs(mcpURL) }
	defer func() {
		hub.StopAll()
		time.Sleep(500 * time.Millisecond)
	}()

	caution.ServeClient()
	if *addr == "" || native.InBundle() {
		mount := mountView(hub, runtime.GOOS == "darwin")
		saved := hub.Settings()
		win := &windowSaver{hub: hub}
		w, h := windowSize(saved)
		err := native.Run(mount, native.Options{Title: "Switchboard", W: w, H: h, Maximized: saved.Maximized,
			OnWindow: win.note, CustomTitlebar: runtime.GOOS == "darwin", TitlebarStyle: "tall"})
		win.flush()
		if err != nil {
			log.Print(err)
		}
		return
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		hub.StopAll()
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
	log.Printf("switchboard listening on %s", *addr)
	log.Fatal(caution.Serve(*addr, mountView(hub, false)))
}

func defaultStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "switchboard", "state.json")
}
