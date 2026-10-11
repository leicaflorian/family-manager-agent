package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework ImageCaptureCore
#include <stdlib.h>
#include "agent_darwin.h"
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

// How long the scanner may stay silent, before the first side and between one and the next.
const scanSilence = 90 * time.Second


// The scanner on the Mac: Image Capture, in scanner_darwin.m.
type darwinDriver struct {
	started time.Time
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

func (d *darwinDriver) Status() string {
	for {
		switch C.fmScannerState() {
		case C.FMScannerReady:
			return ScannerReady
		case C.FMScannerEmpty:
			return ScannerEmpty
		}
		if time.Since(d.started) > openingTime {
			return ScannerOff
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (d *darwinDriver) Scan(dir string, page func(path string)) *ScanError {
	done := make(chan *ScanError, 1)
	sign := make(chan struct{}, 1)
	running.Lock()
	running.page, running.done, running.sign = page, done, sign
	running.Unlock()

	path := C.CString(dir)
	defer C.free(unsafe.Pointer(path))
	if code := int(C.fmScannerScan(path, C.int(Resolution))); code != C.FMScanDone {
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

func (d *darwinDriver) Describe() string {
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

// settle lets Image Capture find the scanner and open it: for the probe, which has no application
// running the main thread.
func settle() {
	C.fmRunLoop(C.double(openingTime.Seconds()))
}
