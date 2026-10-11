package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The program inside the app, as build.sh puts it there.
const bundleProgram = "Contents/MacOS/FamilyManagerAgent"

// updateTarget is the app this agent runs from, "" when it cannot be replaced: a program outside
// an app, or an app macOS runs from a read-only copy (opened where it was downloaded, never moved
// to Applications).
func updateTarget() string {
	program, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(program); err == nil {
		program = resolved
	}
	app := strings.TrimSuffix(program, "/"+bundleProgram)
	if app == program || !strings.HasSuffix(app, ".app") || strings.Contains(app, "/AppTranslocation/") {
		return ""
	}
	return app
}

// restartProgram opens the app again, as a new one: this agent is still quitting.
func restartProgram(target string) error {
	return exec.Command("/usr/bin/open", "-n", target).Start()
}

// replaceProgram opens the zip beside the app, so that the two renames stay on the same disk, and
// puts the new app in the place of the old one. If the new one does not get there the old one
// goes back.
func replaceProgram(zip, target string) error {
	work, err := os.MkdirTemp(filepath.Dir(target), ".FamilyManagerAgent-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	// ditto wrote the zip (build.sh) and reads it back as it was, signature included
	if output, err := exec.Command("/usr/bin/ditto", "-x", "-k", zip, work).CombinedOutput(); err != nil {
		return fmt.Errorf("ditto: %v: %s", err, strings.TrimSpace(string(output)))
	}
	fresh := filepath.Join(work, appName+".app")
	if _, err := os.Stat(filepath.Join(fresh, bundleProgram)); err != nil {
		return errors.New("the zip does not hold the app")
	}
	old := filepath.Join(work, "old.app")
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(fresh, target); err != nil {
		os.Rename(old, target)
		return err
	}
	return nil
}
