package main

import (
	"context"
	"errors"
	"log"
	"time"

	"switchboard/update"
)

var version = "dev"

func (h *Hub) watchUpdates(u update.Updater) {
	if _, ok := update.ParseTag(u.Version); !ok {
		return
	}
	if tag := u.ReadyTag(); tag != "" {
		h.setUpdateReady(tag)
	}
	last := ""
	for {
		if !h.Settings().UpdatesOff {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			tag, err := h.stageUpdate(ctx)
			cancel()
			switch {
			case err != nil:
				if msg := err.Error(); msg != last {
					log.Printf("update check: %v", err)
					last = msg
				}
			case tag != "":
				last = ""
				h.setUpdateReady(tag)
			}
		}
		time.Sleep(time.Hour)
	}
}

func (h *Hub) stageUpdate(ctx context.Context) (string, error) {
	h.stageMu.Lock()
	defer h.stageMu.Unlock()
	return h.updater.CheckAndStage(ctx)
}

var errNoSelfUpdate = errors.New("this build of Switchboard does not update itself")

func (h *Hub) CheckForUpdates(ctx context.Context) (tag string, ready bool, err error) {
	if h.updater == nil {
		return "", false, errNoSelfUpdate
	}
	if h.Settings().UpdatesOff {
		latest, err := h.updater.LatestTag(ctx)
		if err != nil || !update.NewerTag(h.updater.Version, latest) {
			return "", false, err
		}
		return latest, false, nil
	}
	tag, err = h.stageUpdate(ctx)
	if err != nil || tag == "" {
		return "", false, err
	}
	h.setUpdateReady(tag)
	return tag, true, nil
}

func (h *Hub) DownloadUpdate(ctx context.Context) (string, error) {
	if h.updater == nil {
		return "", errNoSelfUpdate
	}
	tag, err := h.stageUpdate(ctx)
	if err == nil && tag == "" {
		err = errors.New("no newer release was found")
	}
	return tag, err
}

var errDevBuild = errors.New("this is a development build of Switchboard, which does not update itself")

func (h *Hub) FetchUpdate(ctx context.Context) (current, tag string, err error) {
	if h.updater == nil {
		if _, ok := update.ParseTag(version); !ok {
			return version, "", errDevBuild
		}
		return version, "", errNoSelfUpdate
	}
	current = h.updater.Version
	if _, ok := update.ParseTag(current); !ok {
		return current, "", errDevBuild
	}
	tag, err = h.stageUpdate(ctx)
	if err != nil || tag == "" {
		return current, "", err
	}
	h.mu.Lock()
	changed := h.updateTag != tag || !h.updateAsked
	h.updateTag = tag
	h.updateAsked = true
	h.mu.Unlock()
	if changed {
		h.each(func(v *View) { v.updateChanged() })
	}
	return current, tag, nil
}

func (h *Hub) setUpdateReady(tag string) {
	h.mu.Lock()
	changed := h.updateTag != tag
	h.updateTag = tag
	h.mu.Unlock()
	if changed {
		h.each(func(v *View) { v.updateChanged() })
	}
}

func (h *Hub) UpdateReady() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settings.UpdatesOff && !h.updateAsked {
		return ""
	}
	return h.updateTag
}

// midTurn names the sessions whose turn a restart would stop.
func (h *Hub) midTurn() []string {
	var names []string
	for _, a := range append([]*Agent{h.Dispatcher}, h.Agents()...) {
		if s := a.Snapshot(); s.State == Working || s.State == Starting {
			names = append(names, s.Meta.Title)
		}
	}
	return names
}

// RestartToUpdate installs the staged release, stops every session, and
// starts the new version. It returns only on failure.
func (h *Hub) RestartToUpdate() error {
	if h.restart == nil {
		return errors.New("this window cannot restart Switchboard")
	}
	return h.restart()
}
