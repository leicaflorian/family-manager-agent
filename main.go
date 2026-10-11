// Family Manager Agent: commands the scanner of this computer on behalf of the page "Scansiona"
// of Family Manager. It starts when the page opens familymanageragent://open, answers on
// 127.0.0.1:47115 only to the sites the owner accepted, and quits after ten minutes of silence.
package main

import (
	_ "embed"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	listener, err := net.Listen("tcp", Address)
	if err != nil {
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
	systray.Run(func() { ready(listener) }, quit)
	// on the Mac the loop ends without calling quit
	quit()
}

func ready(listener net.Listener) {
	driver = newDriver()
	var err error
	if session, err = NewSession(driver); err != nil {
		logf("temporary folder: %v", err)
		tell(appName, "Non riesco a creare la cartella temporanea per le scansioni: "+err.Error())
		systray.Quit()
		return
	}
	origins := LoadOrigins(func(origin string) bool {
		return ask(appName, "Permetti a "+origin+" di usare lo scanner?\n\n"+
			"La pagina potrà far partire lo scanner di questo computer e leggere i fogli scansionati.",
			"Permetti", "Non permettere")
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
	systray.AddSeparator()
	forget := systray.AddMenuItem("Dimentica i siti accettati", "")
	leave := systray.AddMenuItem("Esci", "")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	idle := time.NewTicker(15 * time.Second)
	go func() {
		for {
			select {
			case <-forget.ClickedCh:
				origins.Forget()
				logf("accepted sites forgotten")
			case <-idle.C:
				if !server.Idle() {
					continue
				}
				logf("no request for %s: quitting", idleTime)
				systray.Quit()
				return
			case <-leave.ClickedCh:
				systray.Quit()
				return
			case <-stop:
				systray.Quit()
				return
			}
		}
	}()
}

// quit leaves the scanner to the other programs and deletes the sides still in the temporary folder.
func quit() {
	quitOnce.Do(func() {
		if driver != nil {
			driver.Close()
		}
		if session != nil {
			session.Close()
		}
		logf("quit")
	})
}
