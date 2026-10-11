package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework ImageCaptureCore
#include <stdlib.h>
#include "agent_darwin.h"
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// How long the scanner may stay silent, before the first side and between one and the next.
const scanSilence = 90 * time.Second

// How long after the start the list waits for Image Capture to find the scanners: the ones on the
// cable arrive at once, the ones of the network within a second.
const findingTime = 2 * time.Second

// The scanner on the Mac: Image Capture, in scanner_darwin.m.
type darwinDriver struct {
	started time.Time

	mu      sync.Mutex
	chosen  bool      // a page asked for a scanner
	scanner string    // the one last asked: "" is the page of before
	asked   time.Time // when the page began to ask for it
}

// The scan in progress: where goScanPage and goScanDone, called by Image Capture, send what arrives.
var running struct {
	sync.Mutex
	page func(path string)
	done chan *ScanError
	sign chan struct{}
}

func newDriver() Driver {
	C.fmScannerStart()
	return &darwinDriver{started: time.Now()}
}

func (d *darwinDriver) List() []Scanner {
	if wait := findingTime - time.Since(d.started); wait > 0 && !probing {
		time.Sleep(wait)
	}
	text := C.fmScannerList()
	defer C.free(unsafe.Pointer(text))
	list := []Scanner{}
	for _, line := range strings.Split(C.GoString(text), "\n") {
		if parts := strings.SplitN(line, "\t", 3); len(parts) == 3 {
			list = append(list, Scanner{ID: parts[0], Connection: parts[1], Name: parts[2]})
		}
	}
	return list
}

// choose has Image Capture open the scanner asked, set on the source asked. Asked again for the
// same one it changes nothing.
func (d *darwinDriver) choose(scanner, source string) {
	d.mu.Lock()
	if !d.chosen || scanner != d.scanner {
		d.chosen, d.scanner, d.asked = true, scanner, time.Now()
	}
	d.mu.Unlock()

	unit := C.FMUnitAny
	switch source {
	case SourceFeeder:
		unit = C.FMUnitFeeder
	case SourceFlatbed:
		unit = C.FMUnitFlatbed
	}
	id := C.CString(scanner)
	defer C.free(unsafe.Pointer(id))
	C.fmScannerChoose(id, C.int(unit))
}

func (d *darwinDriver) state() string {
	switch C.fmScannerState() {
	case C.FMScannerReady:
		return ScannerReady
	case C.FMScannerEmpty:
		return ScannerEmpty
	case C.FMScannerOpening:
		return ScannerOpening
	}
	return ScannerOff
}

// open waits for the scanner asked to be open: in the first seconds from when the page asked,
// or from the start of the agent, "off" is not yet an answer.
func (d *darwinDriver) open() string {
	d.mu.Lock()
	asked := d.asked
	d.mu.Unlock()
	for {
		state := d.state()
		if state == ScannerReady || state == ScannerEmpty {
			return state
		}
		if time.Since(asked) > openingTime {
			return ScannerOff
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (d *darwinDriver) Status(scanner, source string) (string, *Capabilities) {
	d.choose(scanner, source)
	if scanner == "" {
		// the page of before waits for the answer, and knows nothing of a scanner that opens
		return d.open(), nil
	}
	state := d.state()
	if state == ScannerOff && time.Since(d.started) < openingTime {
		// Image Capture may not have found it yet
		state = ScannerOpening
	}
	if state != ScannerReady && state != ScannerEmpty {
		return state, nil
	}
	has := d.has()
	if has == nil {
		return ScannerOpening, nil
	}
	return state, has
}

// has is what the scanner open has, or nil while it is not set on a unit.
func (d *darwinDriver) has() *Capabilities {
	text := C.fmScannerHas()
	defer C.free(unsafe.Pointer(text))
	parts := strings.Split(C.GoString(text), "\t")
	if len(parts) != 3 {
		return nil
	}
	// Image Capture does not say which colours a scanner reads: every one seen reads both
	has := &Capabilities{Source: parts[0], Sources: strings.Split(parts[1], ","), Resolutions: []int{},
		Colours: []string{ColourColour, ColourGrey}}
	for _, text := range strings.Split(parts[2], ",") {
		if resolution, err := strconv.Atoi(text); err == nil {
			has.Resolutions = append(has.Resolutions, resolution)
		}
	}
	return has
}

func (d *darwinDriver) Scan(settings Settings, dir string, page func(path string)) *ScanError {
	d.choose(settings.Scanner, settings.Source)
	if d.open() == ScannerOff {
		return &ScanError{Code: ErrorOff}
	}
	if has := d.has(); has != nil && has.Source != settings.Source {
		return &ScanError{Code: ErrorScanner, Message: "the scanner has no " + settings.Source}
	}

	done := make(chan *ScanError, 1)
	sign := make(chan struct{}, 1)
	running.Lock()
	running.page, running.done, running.sign = page, done, sign
	running.Unlock()

	path := C.CString(dir)
	defer C.free(unsafe.Pointer(path))
	grey := 0
	if settings.Colour == ColourGrey {
		grey = 1
	}
	switch code := int(C.fmScannerScan(path, C.int(settings.Resolution), C.int(grey))); code {
	case C.FMScanDone:
	case C.FMScanSettings:
		return &ScanError{Code: ErrorScanner, Message: fmt.Sprintf("the scanner does not have %d dots", settings.Resolution)}
	default:
		return scanError(code, "")
	}
	for {
		select {
		case err := <-done:
			return err
		case <-sign:
		case <-time.After(scanSilence):
			C.fmScannerCancel()
			return &ScanError{Code: ErrorScanner, Message: "the scanner gives no sign of life"}
		}
	}
}

// Describe says what Image Capture sees and did. What a scanner has is there once the scanner is
// open: the probe asks for its status first.
func (d *darwinDriver) Describe(scanner string) string {
	text := C.fmScannerDescribe()
	defer C.free(unsafe.Pointer(text))
	return C.GoString(text)
}

func (d *darwinDriver) Close() {
	C.fmScannerStop()
}

func scanError(code int, message string) *ScanError {
	switch code {
	case C.FMScanDone:
		return nil
	case C.FMScanOff:
		return &ScanError{Code: ErrorOff, Message: message}
	case C.FMScanEmpty:
		return &ScanError{Code: ErrorEmpty, Message: message}
	case C.FMScanJam:
		return &ScanError{Code: ErrorJam, Message: message}
	}
	return &ScanError{Code: ErrorScanner, Message: message}
}

//export goScanPage
func goScanPage(path *C.char) {
	running.Lock()
	page, sign := running.page, running.sign
	running.Unlock()
	if page == nil {
		return
	}
	page(C.GoString(path))
	select {
	case sign <- struct{}{}:
	default:
	}
}

//export goScanDone
func goScanDone(code C.int, message *C.char) {
	running.Lock()
	done := running.done
	running.page, running.done, running.sign = nil, nil, nil
	running.Unlock()
	if done != nil {
		done <- scanError(int(code), C.GoString(message))
	}
}

// settle runs the main thread, where Image Capture finds the scanners and opens them: for the
// probe, which has no application running it.
func settle(wait time.Duration) {
	C.fmRunLoop(C.double(wait.Seconds()))
}
