package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// probing is true in the probe, which waits for the scanner as long as it takes: nobody is
// waiting for it as the page waits for the agent.
var probing bool

// probe writes what the system sees of the scanner, without feeding a sheet: on the screen and in
// scanner-probe.txt, next to the program. It is the first thing to run on a computer never tried.
func probe() {
	probing = true
	driver := newDriver()
	settle()
	text := fmt.Sprintf("Family Manager Agent %s, probe of %s\n%s/%s\n\n%s\nstatus: %s\n",
		Version, time.Now().Format("2006-01-02 15:04"), runtime.GOOS, runtime.GOARCH, driver.Describe(), driver.Status())
	driver.Close()

	fmt.Print(text)
	if exe, err := os.Executable(); err == nil {
		os.WriteFile(filepath.Join(filepath.Dir(exe), "scanner-probe.txt"), []byte(text), 0o644)
	}
}
