package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// The name of the program in its own folder, and of the link that starts it (familymanageragent://open).
const (
	programName = "Family Manager Agent.exe"
	scheme      = "familymanageragent"
)

// install makes the link familymanageragent://open start the agent, without an installer: at every
// start the program copies itself in %LOCALAPPDATA%\FamilyManagerAgent (unless it runs from there)
// and writes the scheme in the registry of the user. Opening it once is enough.
func install() {
	current, err := os.Executable()
	if err != nil {
		logf("install: %v", err)
		return
	}
	target, err := installPath()
	if err != nil {
		logf("install: %v", err)
		return
	}
	// the program an update moved aside (see replaceProgram): no agent runs from it any more
	os.Remove(target + oldSuffix)
	if !strings.EqualFold(filepath.Clean(current), filepath.Clean(target)) {
		// the copy fails while an agent started from there is running: the one already there stays
		if err := copyFile(current, target); err != nil {
			logf("install: copy: %v", err)
			if _, missing := os.Stat(target); missing != nil {
				target = current
			}
		}
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.SET_VALUE)
	if err != nil {
		logf("install: registry: %v", err)
		return
	}
	defer key.Close()
	key.SetStringValue("", "URL:Family Manager Agent")
	key.SetStringValue("URL Protocol", "")
	command, _, err := registry.CreateKey(key, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		logf("install: registry: %v", err)
		return
	}
	defer command.Close()
	command.SetStringValue("", `"`+target+`" "%1"`)
}

// installPath is where the program lives: %LOCALAPPDATA%\FamilyManagerAgent.
func installPath() (string, error) {
	base, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "FamilyManagerAgent", programName), nil
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(to)
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}
