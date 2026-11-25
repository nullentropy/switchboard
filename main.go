package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

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
	prepareApp()

	bins := map[string]string{"claude": *claudeBin, "codex": *codexBin, "gemini": *geminiBin}
	if *demo {
		*statePath = ""
		bins = map[string]string{"claude": "true", "codex": "true", "gemini": "true"}
	}
	hub := newHub(bins, *statePath)
	var flushWindow func()
	if !*demo {
		startUpdates(hub, &flushWindow)
	}
	go hub.loadCodexModels()
	go hub.loadClaudeModels()
	if err := checkBin(hub, "claude"); err != nil && !*demo {
		log.Fatal("claude is not on PATH; pass -claude /path/to/claude")
	}
	if *demo {
		hub.runDemo(*demoN)
	} else {
		go hub.watchLoad()
		go hub.watchLimits()
	}
	mcpURL, err := hub.startMCP()
	if err != nil {
		log.Fatalf("start the dispatcher's tool server: %v", err)
	}
	hub.Dispatcher.extraArgs = func() []string { return dispatcherArgs(mcpURL) }
	defer hub.StopAll()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		hub.StopAll()
		os.Exit(0)
	}()

	caution.ServeClient()
	if *addr == "" || native.InBundle() {
		mount := mountView(hub, runtime.GOOS == "darwin")
		saved := hub.Settings()
		win := &windowSaver{hub: hub}
		flushWindow = win.flush
		w, h := windowSize(saved)
		err := native.Run(mount, native.Options{Title: "Switchboard", W: w, H: h, Maximized: saved.Maximized,
			OnWindow: win.note, AppID: "switchboard", CustomTitlebar: runtime.GOOS == "darwin", TitlebarStyle: "tall"})
		win.flush()
		if err != nil {
			log.Print(err)
		}
		return
	}

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

// startUpdates installs a staged release before anything else starts, then
// keeps checking for newer ones.
func startUpdates(hub *Hub, flushWindow *func()) {
	u := newUpdater()
	if _, ok := parseTag(u.version); !ok {
		return
	}
	root, exe, err := installRoot()
	if err != nil {
		log.Printf("update: find the install: %v", err)
		return
	}
	if ok, err := u.atLaunch(hub.Settings(), root); err != nil {
		log.Printf("update: %v", err)
	} else if ok {
		log.Printf("update: installed, relaunching")
		if err := relaunch(exe); err != nil {
			log.Printf("update: relaunch: %v", err)
		}
	}
	hub.restart = func() error {
		ok, err := u.installStaged(root)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("no update is ready")
		}
		if *flushWindow != nil {
			(*flushWindow)()
		}
		hub.StopAll()
		return relaunch(exe)
	}
	go hub.watchUpdates(u)
}
