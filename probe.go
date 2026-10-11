package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// probing is true in the probe, which waits for the scanner as long as it takes: nobody is
// waiting for it as the page waits for the agent.
var probing bool

// How long the probe waits for the scanner it was asked to open.
const probeWait = 30 * time.Second

// probe writes what the system sees of the scanners, without feeding a sheet: on the screen and in
// scanner-probe.txt, next to the program. It is the first thing to run on a computer never tried.
// Alone it lists the scanners and opens none; with the id of one of them it opens that one and
// says what it has.
func probe() {
	probing = true
	scanner := ""
	for _, argument := range os.Args[1:] {
		if !strings.HasPrefix(argument, "--") {
			scanner = argument
		}
	}
	driver := newDriver()
	settle(openingTime)

	var text strings.Builder
	fmt.Fprintf(&text, "Family Manager Agent %s, probe of %s\n%s/%s\n\n", Version, time.Now().Format("2006-01-02 15:04"),
		runtime.GOOS, runtime.GOARCH)
	list := driver.List()
	fmt.Fprintf(&text, "scanners: %d\n", len(list))
	for _, found := range list {
		fmt.Fprintf(&text, "- %s (%s), id %s\n", found.Name, found.Connection, found.ID)
	}
	if scanner == "" {
		fmt.Fprintf(&text, "\nNone was opened. To see what one of them has: %s \"<id>\"\n", filepath.Base(os.Args[0]))
	} else {
		state, has := driver.Status(scanner, "")
		for started := time.Now(); state == ScannerOpening && time.Since(started) < probeWait; {
			settle(time.Second)
			state, has = driver.Status(scanner, "")
		}
		fmt.Fprintf(&text, "\nasked: %s\nstatus: %s\n", scanner, state)
		if has != nil {
			fmt.Fprintf(&text, "set on: %s\nsources: %v\nresolutions: %v\ncolours: %v\n", has.Source, has.Sources,
				has.Resolutions, has.Colours)
		}
	}
	fmt.Fprintf(&text, "\n%s", driver.Describe(scanner))
	driver.Close()

	fmt.Print(text.String())
	if exe, err := os.Executable(); err == nil {
		os.WriteFile(filepath.Join(filepath.Dir(exe), "scanner-probe.txt"), []byte(text.String()), 0o644)
	}
}
