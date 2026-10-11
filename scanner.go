package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// What the scanner is doing, as the page reads it in GET /status.
const (
	ScannerReady = "ready" // on, with sheets in the feeder, or with a glass to put the sheet on
	ScannerEmpty = "empty" // on, the feeder is empty
	ScannerOff   = "off"   // off, not attached, not the one asked, or without a feeder
	// the scanner asked is being opened: only the page that chooses the scanner reads it (api 2).
	// The page of before reads "off" in its place
	ScannerOpening = "opening"
)

// Where the sheets are taken from, how they are read and how the scanner is reached, as the page
// writes and reads them.
const (
	SourceFeeder  = "feeder"
	SourceFlatbed = "flatbed"

	ColourColour = "colour"
	ColourGrey   = "grey"

	ConnectionUSB     = "usb"
	ConnectionNetwork = "network"
)

// The state of a scan, as the page reads it in GET /pages.
const (
	StateIdle     = "idle"
	StateScanning = "scanning"
	StateDone     = "done"
	StateError    = "error"
)

// Why a scan stopped. The page has a message for each code.
const (
	ErrorOff     = "off"     // the scanner is off or was detached
	ErrorEmpty   = "empty"   // no sheets in the feeder
	ErrorJam     = "jam"     // a sheet is stuck
	ErrorScanner = "scanner" // anything else: Message holds what the driver said
)

// Scanner is one of the scanners the system sees, as GET /scanners lists them.
type Scanner struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Connection string `json:"connection"`
}

// Capabilities is what the scanner asked has, read with the scanner open: where it takes the
// sheets from, and the resolutions and the colours of the source it is set on.
type Capabilities struct {
	Source      string   `json:"source"`
	Sources     []string `json:"sources"`
	Resolutions []int    `json:"resolutions"`
	Colours     []string `json:"colours"`
}

// Has says whether the settings are among what the scanner has.
func (c *Capabilities) Has(settings Settings) bool {
	source, resolution, colour := false, false, false
	for _, has := range c.Sources {
		source = source || has == settings.Source
	}
	for _, has := range c.Resolutions {
		resolution = resolution || has == settings.Resolution
	}
	for _, has := range c.Colours {
		colour = colour || has == settings.Colour
	}
	return source && resolution && colour
}

// Settings is what the page chose for a scan. Always an A4, one JPEG per side, and both sides
// from the feeder; one side from the glass.
type Settings struct {
	// Scanner is the id of a scanner of the list. "" is the page of before, which chooses nothing:
	// the first scanner on the cable with a feeder
	Scanner    string
	Source     string
	Resolution int
	Colour     string
}

// Before is what the page of before gets, which sends no settings: the feeder of the first scanner
// on the cable, 200 dots, colour.
var Before = Settings{Source: SourceFeeder, Resolution: 200, Colour: ColourColour}

// A4 is the size of the sheet in dots: 1240 x 1754 at 150, 1654 x 2339 at 200 (about the size of
// the photos of Family Scan), 2480 x 3508 at 300.
func A4(resolution int) (width, height int) {
	return (resolution*2100 + 127) / 254, (resolution*2970 + 127) / 254
}

// How long after the start the scanner may still be opening (on the Mac the session opens in a
// couple of seconds): until then "off" is not yet an answer.
const openingTime = 6 * time.Second

// ScanError is a scan that stopped before the end of the sheets.
type ScanError struct {
	Code    string
	Message string
}

func (e *ScanError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Driver is the part that commands the scanner, written once per system
// (scanner_darwin.go with Image Capture, scanner_windows.go with WIA).
type Driver interface {
	// List says the scanners the system sees, the ones of the network too, without opening any.
	List() []Scanner
	// Status opens the scanner asked, and only that one, and says how it is: ScannerReady,
	// ScannerEmpty, ScannerOff, or ScannerOpening with nothing else while it opens. The scanner
	// opened before is let go. source is where the sheets will come from, or "" for the feeder
	// when the scanner has one and the glass when it does not. With scanner "" it is the page of
	// before: the first scanner on the cable with a feeder, never ScannerOpening, no capabilities.
	Status(scanner, source string) (string, *Capabilities)
	// Scan feeds every sheet of the feeder, or reads the one on the glass, and returns when they
	// are over. Each side is written as a JPEG in dir and handed to page, in the order of
	// arrival. The file is then the caller's.
	Scan(settings Settings, dir string, page func(path string)) *ScanError
	// Describe says in plain text what the system sees, and with a scanner what that one has:
	// for the probe and the log. Without a scanner it opens none.
	Describe(scanner string) string
	// Close leaves the scanner free for the other programs.
	Close()
}

// Page is one side of a sheet, waiting in the temporary folder.
type Page struct {
	ID   string `json:"id"`
	Size int64  `json:"size"`
	path string
}

// Session holds the sides scanned so far. They live in a temporary folder until the page uploads
// them (DELETE /pages) or the agent quits: no file is left on the computer.
type Session struct {
	mu     sync.Mutex
	driver Driver
	// ID changes at every start of the agent: the ids of the sides start again from 1, and the page
	// must not take a new side for one it already has
	ID  string
	dir string
	state  string
	err    *ScanError
	pages  []Page
	next   int
}

// sessionFolder is how the temporary folder of the sides begins, in the temporary folder of the user.
const sessionFolder = "FamilyManagerAgent-"

func NewSession(driver Driver) (*Session, error) {
	dir, err := os.MkdirTemp("", sessionFolder)
	if err != nil {
		return nil, err
	}
	return &Session{driver: driver, ID: fmt.Sprintf("%x", time.Now().UnixNano()), dir: dir, state: StateIdle}, nil
}

// Start feeds the sheets in the background and queues them after the sides already there.
// It is false when a scan is already running.
func (s *Session) Start(settings Settings) bool {
	s.mu.Lock()
	if s.state == StateScanning {
		s.mu.Unlock()
		return false
	}
	s.state = StateScanning
	s.err = nil
	s.mu.Unlock()

	go func() {
		err := s.driver.Scan(settings, s.dir, s.add)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.state = StateError
			s.err = err
			logf("scan stopped: %v", err)
			return
		}
		s.state = StateDone
	}()
	return true
}

// add takes a file written by the driver and gives it its place in the list.
func (s *Session) add(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("%d", s.next)
	target := filepath.Join(s.dir, id+".jpg")
	if path != target {
		if err := os.Rename(path, target); err != nil {
			logf("page %s: %v", id, err)
			return
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		logf("page %s: %v", id, err)
		return
	}
	s.pages = append(s.pages, Page{ID: id, Size: info.Size(), path: target})
}

// Snapshot is the answer of GET /pages.
func (s *Session) Snapshot() (state string, err *ScanError, pages []Page) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.err, append([]Page{}, s.pages...)
}

func (s *Session) Scanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == StateScanning
}

// Path is the file of a side, or "" when the id is unknown.
func (s *Session) Path(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, page := range s.pages {
		if page.ID == id {
			return page.path
		}
	}
	return ""
}

// Clear deletes every side. It is false while a scan is running.
func (s *Session) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateScanning {
		return false
	}
	s.empty()
	s.pages = nil
	s.state = StateIdle
	s.err = nil
	return true
}

// Close deletes the temporary folder: called when the agent quits.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.RemoveAll(s.dir)
	s.pages = nil
}

func (s *Session) empty() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		os.RemoveAll(filepath.Join(s.dir, entry.Name()))
	}
}
