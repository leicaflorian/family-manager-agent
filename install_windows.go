package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The name of the program in its own folder, and of the link that starts it (familymanageragent://open).
const (
	programName = "Family Manager Agent.exe"
	scheme      = "familymanageragent"
)

// install makes the link familymanageragent://open start the agent, without an installer: at every
// start the program copies itself in %LOCALAPPDATA%\FamilyManagerAgent (unless it runs from there)
// and writes the scheme in the registry of the user. Opening it once is enough. It is true when
// this start put the program there and the link starts it: the owner opened the downloaded file,
// and has to be told that it worked.
func install() bool {
	current, err := os.Executable()
	if err != nil {
		logf("install: %v", err)
		return false
	}
	target, err := installPath()
	if err != nil {
		logf("install: %v", err)
		return false
	}
	copied := false
	// the program an update moved aside (see replaceProgram): no agent runs from it any more
	os.Remove(target + oldSuffix)
	if !strings.EqualFold(filepath.Clean(current), filepath.Clean(target)) {
		// the copy fails while an agent started from there is running: the one already there stays
		if err := copyFile(current, target); err != nil {
			logf("install: copy: %v", err)
			if _, missing := os.Stat(target); missing != nil {
				target = current
			}
		} else {
			copied = true
			logf("install: copied in %s", target)
		}
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.SET_VALUE)
	if err != nil {
		logf("install: registry: %v", err)
		return false
	}
	defer key.Close()
	key.SetStringValue("", "URL:Family Manager Agent")
	key.SetStringValue("URL Protocol", "")
	command, _, err := registry.CreateKey(key, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		logf("install: registry: %v", err)
		return false
	}
	defer command.Close()
	if err := command.SetStringValue("", `"`+target+`" "%1"`); err != nil {
		logf("install: registry: %v", err)
		return false
	}
	return copied
}

// tellInstalled says in a window that the program is in its folder, and what to do next, which is
// what the page says too: without it nothing shows that opening the downloaded file worked, the
// icon being easy to miss. It is a message, not a warning, and waits for OK.
func tellInstalled() {
	caption, _ := windows.UTF16PtrFromString(appName)
	message, _ := windows.UTF16PtrFromString("Installazione riuscita: la versione " + Version + " è su questo computer.\n\n" +
		"Torna alla pagina Scansiona di Family Manager e premi Riprova. Da adesso la pagina avvia l'agente da sola, " +
		"e l'agente si chiude quando non serve.\n\n" +
		"Finché è aperto la sua icona è accanto all'orologio, a volte dietro la freccia ^. " +
		"Il file scaricato non serve più.")
	windows.MessageBox(0, message, caption, boxInfo|boxForeground|boxTopmost)
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
