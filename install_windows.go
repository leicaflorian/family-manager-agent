package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

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

// takeOver is called when another agent holds the port. Started by the link, this one has nothing
// to do: the page opened the link twice. Opened by hand it is the downloaded file, which cannot
// copy itself over the program that is open: the owner is asked whether to close that one, and on
// yes it is ended without waiting for it. It is true when this agent can go on in its place.
func takeOver() bool {
	if byLink() {
		return false
	}
	if !ask(appName, "Family Manager Agent è già aperto su questo computer.\n\n"+
		"Per installare questa copia (versione "+Version+") quello aperto va chiuso. Lo chiudo adesso?\n\n"+
		"Una scansione in corso viene interrotta.", "Chiudi", "Annulla") {
		return false
	}
	closed, err := closeAgents()
	if err != nil || closed == 0 {
		reason := ""
		if err != nil {
			reason = "\n\nIl motivo: " + err.Error()
		}
		tell(appName, "Non sono riuscito a chiudere l'agente aperto.\n\n"+
			"Scegli Esci dalla sua icona, accanto all'orologio, e riapri questo file."+reason)
		return false
	}
	// an agent ended that way did not delete the sides it still had: no other agent is open now
	leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), sessionFolder+"*"))
	for _, folder := range leftovers {
		os.RemoveAll(folder)
	}
	return true
}

// byLink is true when the program was started by the link familymanageragent://open, which
// Windows gives it as its argument, and not opened by hand.
func byLink() bool {
	return len(os.Args) > 1 && strings.HasPrefix(strings.ToLower(os.Args[1]), scheme+":")
}

// How long an agent told to end is waited for, in milliseconds.
const closingTime = 5000

// closeAgents ends the other agents open on this computer, and answers how many: the programs
// with the name of the installed one, or of this file, that are not this one. They do not run
// their quit.
func closeAgents() (int, error) {
	own := programName
	if current, err := os.Executable(); err == nil {
		own = filepath.Base(current)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	closed := 0
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if entry.ProcessID == windows.GetCurrentProcessId() || !(strings.EqualFold(name, programName) || strings.EqualFold(name, own)) {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, entry.ProcessID)
		if err != nil {
			return closed, err
		}
		err = windows.TerminateProcess(process, 0)
		if err == nil {
			// the port and the file of the program are free only when it has ended
			var event uint32
			if event, err = windows.WaitForSingleObject(process, closingTime); err == nil && event != windows.WAIT_OBJECT_0 {
				err = windows.WAIT_TIMEOUT
			}
		}
		windows.CloseHandle(process)
		if err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
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
