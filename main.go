// Family Manager Agent: commands the scanner of this computer on behalf of the page "Scansiona"
// of Family Manager. It starts when the page opens familymanageragent://open, answers on
// 127.0.0.1:47115 only to the sites the owner accepted, and quits after ten minutes of silence.
// While it is open it looks for a newer version among its releases, and installs it when it quits,
// or at once when the owner asks from the menu.
package main

import (
	_ "embed"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
)

const appName = "Family Manager Agent"

//go:embed VERSION
var versionFile string

// Version is the one of the file VERSION, which build.sh also writes in version.json.
var Version = strings.TrimSpace(versionFile)

// mode is "probe" in the program built by "build.sh probe" (-ldflags "-X main.mode=probe").
var mode string

var (
	driver   Driver
	session  *Session
	updater  *Updater
	listener net.Listener
	// true when the agent quits to start again as the version it has just installed
	restart atomic.Bool
	// true from the moment the agent starts quitting
	leaving  atomic.Bool
	quitOnce sync.Once
)

func init() {
	// the main thread is the one of the windows and, on the Mac, of Image Capture
	runtime.LockOSThread()
}

func logf(format string, arguments ...any) {
	log.Printf(format, arguments...)
}

// openLog sends the log to agent.log, in the settings folder, started again at every launch. It
// holds what the scanner and the sites did, never a scanned sheet.
func openLog() {
	file, err := os.Create(filepath.Join(configDir(), "agent.log"))
	if err != nil {
		return
	}
	log.SetOutput(file)
}

func main() {
	if mode == "probe" || (len(os.Args) > 1 && os.Args[1] == "--probe") {
		probe()
		return
	}
	var err error
	if listener, err = net.Listen("tcp", Address); err != nil {
		// an agent is already open (the page opened the link twice): this one has nothing to do,
		// and leaves the log of the other alone
		if otherAgent() {
			return
		}
		tell(appName, "La porta "+Port+" è usata da un altro programma: l'agente non può partire.\n\nChiudi quel programma e riapri l'agente.")
		os.Exit(1)
	}
	openLog()
	logf("%s %s on %s/%s", appName, Version, runtime.GOOS, runtime.GOARCH)
	install()
	systray.Run(ready, quit)
	// on the Mac the loop ends without calling quit
	quit()
}

func ready() {
	driver = newDriver()
	var err error
	if session, err = NewSession(driver); err != nil {
		logf("temporary folder: %v", err)
		tell(appName, "Non riesco a creare la cartella temporanea per le scansioni: "+err.Error())
		systray.Quit()
		return
	}
	origins := LoadOrigins(func(origin string) bool {
		allowed := ask(appName, "Permetti a "+origin+" di usare lo scanner?\n\n"+
			"La pagina potrà far partire lo scanner di questo computer e leggere i fogli scansionati.",
			"Permetti", "Non permettere")
		if leaving.Load() {
			// the question was closed by the agent quitting, not by the owner: no answer to keep
			select {}
		}
		return allowed
	})
	server := NewServer(session, origins)
	go func() {
		logf("server: %v", server.Serve(listener))
	}()

	// what the system sees of the scanner, once: the first thing to read when a scan does not start
	go func() {
		time.Sleep(openingTime)
		logf("scanner:\n%s", driver.Describe())
	}()

	if runtime.GOOS == "darwin" {
		systray.SetTemplateIcon(templateIcon(), templateIcon())
	} else {
		systray.SetIcon(windowsIcon())
	}
	systray.SetTooltip(appName)
	title := systray.AddMenuItem(appName+" "+Version, "")
	title.Disable()
	// shown when a newer version is downloaded and waits for the agent to quit
	waiting := systray.AddMenuItem("", "")
	waiting.Disable()
	waiting.Hide()
	about := systray.AddMenuItem("Informazioni", "")
	// where the window of Informazioni has no buttons of its own, the two stay in the menu; a
	// channel left empty is never chosen below
	update := &updateButton{}
	var updateClicked, forgetClicked chan struct{}
	if !windowButtons {
		update.entry = systray.AddMenuItem("", "")
		updateClicked = update.entry.ClickedCh
		systray.AddSeparator()
		forgetClicked = systray.AddMenuItem(forgetTitle, "").ClickedCh
	} else {
		systray.AddSeparator()
	}
	update.SetTitle(updateTitle)
	leave := systray.AddMenuItem("Esci", "")

	updater = NewUpdater(func(version string) {
		waiting.SetTitle("Versione " + version + " pronta: si installa alla chiusura")
		waiting.Show()
		update.SetTitle("Installa adesso la versione " + version)
	})
	go updater.Run()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	idle := time.NewTicker(15 * time.Second)
	go func() {
		for {
			select {
			case <-forgetClicked:
				origins.Forget()
				logf("accepted sites forgotten")
			case <-about.ClickedCh:
				// in its own thread: the window stays open until a button is clicked
				go showAbout(about, origins, update, waiting)
			case <-updateClicked:
				// in its own thread: the check can take as long as the download
				go updateNow(update, waiting)
			case <-idle.C:
				if !server.Idle() {
					continue
				}
				logf("no request for %s: quitting", idleTime)
				shutdown()
				return
			case <-leave.ClickedCh:
				shutdown()
				return
			case <-stop:
				shutdown()
				return
			}
		}
	}()
}

// What the owner clicks to look for a newer version now, and to have every site asked about again.
const (
	updateTitle = "Cerca aggiornamenti"
	forgetTitle = "Dimentica i siti accettati"
)

// updateButton is what the owner clicks to have a newer version now: a button of the window of
// Informazioni, or an entry of the menu where that window has no buttons. Once a version is
// downloaded it carries its number.
type updateButton struct {
	title atomic.Value      // string
	entry *systray.MenuItem // nil when the button is in the window
}

func (b *updateButton) Title() string {
	return b.title.Load().(string)
}

func (b *updateButton) SetTitle(title string) {
	b.title.Store(title)
	if b.entry != nil {
		b.entry.SetTitle(title)
	}
}

// showAbout is the entry Informazioni of the menu: a window that says what the agent is, which
// version this one is and which sites the owner answered about. Where the window can carry them
// it also has the buttons that look for a newer version and forget the sites.
func showAbout(entry *systray.MenuItem, origins *Origins, update *updateButton, waiting *systray.MenuItem) {
	entry.Disable()
	defer entry.Enable()
	for {
		allowed, denied := origins.Answers()
		buttons := []string{"OK"}
		if windowButtons {
			buttons = append(buttons, update.Title())
			if len(allowed)+len(denied) > 0 {
				buttons = append(buttons, forgetTitle)
			}
		}
		// the button that looks for a newer version leaves the window open: the answer comes in
		// a window above it
		place := chooseOpen(appName, aboutText(allowed, denied), buttons, 1, func() string {
			updateNow(update, waiting)
			return update.Title()
		})
		if place != 2 {
			return
		}
		origins.Forget()
		logf("accepted sites forgotten")
		// the window again: the list is now empty
	}
}

func aboutText(allowed, denied []string) string {
	text := "Versione " + Version + "\n\n" +
		"Comanda lo scanner attaccato a questo computer per conto della pagina Scansiona di Family Manager: " +
		"fa passare i fogli del caricatore e li dà alla pagina, che li carica.\n\n" +
		"Risponde solo ai siti che hai accettato. Si chiude da solo dopo dieci minuti senza richieste, " +
		"e sul computer non lascia nessuna scansione.\n\n"
	if len(allowed) == 0 {
		text += "Nessun sito accettato: al primo che chiama, l'agente chiede."
	} else {
		text += "Siti accettati:\n" + strings.Join(allowed, "\n")
	}
	if len(denied) > 0 {
		text += "\n\nSiti rifiutati:\n" + strings.Join(denied, "\n")
	}
	return text
}

// updateNow looks for a newer version now: the check the agent makes by itself, and the new
// version installed at once instead of when the agent quits. The agent then starts again. The
// answer is always a window: the owner asked.
func updateNow(button *updateButton, waiting *systray.MenuItem) {
	if button.entry != nil {
		button.entry.Disable()
		defer button.entry.Enable()
	}
	found, version, err := updater.Check()
	switch {
	case errors.Is(err, errFixed):
		tell(appName, "Questa copia dell'agente non può aggiornarsi da sola.\n\nSposta Family Manager Agent nella cartella Applicazioni e riaprilo.")
		return
	case err != nil:
		logf("update: %v", err)
		tell(appName, "Non riesco a cercare gli aggiornamenti.\n\nControlla la connessione e riprova. Il motivo è scritto in agent.log.")
		return
	case found == updateNone:
		tell(appName, "Questa è già l'ultima versione ("+Version+").")
		return
	case found == updateLater:
		tell(appName, "La versione "+version+" non si installa da qui: si scarica dalla pagina Scansiona di Family Manager, che la offre quando è pronta per lei.")
		return
	case session.Scanning():
		tell(appName, "La versione "+version+" è pronta, ma c'è una scansione in corso.\n\nVerrà installata quando l'agente si chiude.")
		return
	}
	// installed here and not in quit: if it does not get there the agent is still open to say so
	if _, err := updater.Install(); err != nil {
		button.SetTitle(updateTitle)
		waiting.Hide()
		tell(appName, "Non sono riuscito a installare la versione "+version+".\n\nL'agente resta alla "+Version+". Il motivo è scritto in agent.log.")
		return
	}
	restart.Store(true)
	shutdown()
}

// shutdown quits the agent, after closing the windows that are open: see closeWindows.
func shutdown() {
	leaving.Store(true)
	closeWindows()
	systray.Quit()
}

// quit leaves the scanner to the other programs, deletes the sides still in the temporary folder
// and, when a newer version was downloaded, puts it in the place of this one. After an update
// asked from the menu it starts the agent again.
func quit() {
	quitOnce.Do(func() {
		if driver != nil {
			driver.Close()
		}
		if session != nil {
			session.Close()
		}
		if updater != nil {
			updater.Install()
		}
		if restart.Load() {
			// the port first: the new agent quits at once when it finds another one on it
			listener.Close()
			if err := restartProgram(updateTarget()); err != nil {
				logf("update: restart: %v", err)
			}
		}
		logf("quit")
	})
}
