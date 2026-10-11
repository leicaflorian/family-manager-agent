package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// feeder stands for the scanner. A scan writes a file for each side the test hands it, and goes
// on until the test says the sheets are over.
type feeder struct {
	mu      sync.Mutex
	status  string
	asked   int         // how many times the status was asked
	scans   int         // how many scans started
	closed  bool        // the scanner was left to the other programs
	sheets  chan string // what is on each side of the scan in progress
	failure *ScanError  // how the scan in progress ends

	list     []Scanner     // the scanners the system sees
	has      *Capabilities // what the scanner asked by its id has, once open
	listed   int           // how many times the list was asked
	opened   []string      // the scanner and the source of every status asked, as "scanner/source"
	settings []Settings    // the settings of every scan started
}

func newFeeder() *feeder {
	return &feeder{status: ScannerReady, sheets: make(chan string)}
}

func (f *feeder) List() []Scanner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed++
	return f.list
}

// Status is what the test set, with what the scanner has only for the page that asks for one by
// its id, and only when it is open.
func (f *feeder) Status(scanner, source string) (string, *Capabilities) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	f.opened = append(f.opened, scanner+"/"+source)
	if scanner == "" || f.status == ScannerOff || f.status == ScannerOpening {
		return f.status, nil
	}
	return f.status, f.has
}

func (f *feeder) Scan(settings Settings, dir string, page func(path string)) *ScanError {
	f.mu.Lock()
	f.scans++
	f.settings = append(f.settings, settings)
	scan, sheets := f.scans, f.sheets
	f.mu.Unlock()
	side := 0
	for content := range sheets {
		side++
		path := filepath.Join(dir, fmt.Sprintf("scan-%d-side-%d.tmp", scan, side))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return &ScanError{Code: ErrorScanner, Message: err.Error()}
		}
		page(path)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	failure := f.failure
	f.failure, f.sheets = nil, make(chan string)
	return failure
}

func (f *feeder) Describe(scanner string) string { return "the scanner of the tests" }

func (f *feeder) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// feed passes a side through the scanner.
func (f *feeder) feed(content string) {
	f.mu.Lock()
	sheets := f.sheets
	f.mu.Unlock()
	sheets <- content
}

// finish ends the scan in progress: the sheets are over, or the scanner stopped.
func (f *feeder) finish(failure *ScanError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = failure
	close(f.sheets)
}

func (f *feeder) count() (asked, scans int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked, f.scans
}

// sessionWith is a session with its sides in a folder of the test.
func sessionWith(t *testing.T, driver Driver) *Session {
	t.Helper()
	return &Session{driver: driver, ID: "test", dir: t.TempDir(), state: StateIdle}
}

// scanAll starts a scan, feeds the sides given and waits for its end.
func scanAll(t *testing.T, session *Session, scanner *feeder, failure *ScanError, sides ...string) {
	t.Helper()
	if !session.Start(Before) {
		t.Fatal("the scan does not start")
	}
	for _, side := range sides {
		scanner.feed(side)
	}
	scanner.finish(failure)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })
}

// ids and sizes of the sides a session holds, as "id:size".
func sidesOf(session *Session) []string {
	_, _, pages := session.Snapshot()
	sides := []string{}
	for _, page := range pages {
		sides = append(sides, fmt.Sprintf("%s:%d", page.ID, page.Size))
	}
	return sides
}

// files lists what is in the folder of a session.
func files(t *testing.T, session *Session) []string {
	t.Helper()
	entries, err := os.ReadDir(session.dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestScanErrorSaysTheCodeAndTheMessage(t *testing.T) {
	if text := (&ScanError{Code: ErrorEmpty}).Error(); text != "empty" {
		t.Errorf("%q", text)
	}
	if text := (&ScanError{Code: ErrorScanner, Message: "the driver said no"}).Error(); text != "scanner: the driver said no" {
		t.Errorf("%q", text)
	}
}

func TestNewSessionHasItsOwnEmptyFolder(t *testing.T) {
	// the temporary folder of the user, for the Mac and for Windows: not the one the agent uses
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	t.Setenv("TMP", temporary)

	session, err := NewSession(newFeeder())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(session.dir) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(session.dir), sessionFolder) {
		t.Errorf("the folder is %s", session.dir)
	}
	if left := files(t, session); len(left) != 0 {
		t.Errorf("in the folder: %q", left)
	}
	state, failure, pages := session.Snapshot()
	if session.ID == "" || state != StateIdle || failure != nil || len(pages) != 0 {
		t.Errorf("id %q, state %s, error %v, %d sides", session.ID, state, failure, len(pages))
	}

	session.Close()
	if _, err := os.Stat(session.dir); !os.IsNotExist(err) {
		t.Errorf("after Close the folder is still there: %v", err)
	}
}

func TestScanQueuesTheSidesAsTheyArrive(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	if !session.Start(Before) {
		t.Fatal("the scan does not start")
	}
	if state, _, pages := session.Snapshot(); state != StateScanning || len(pages) != 0 {
		t.Errorf("at the start: %s, %d sides", state, len(pages))
	}

	// the page reads the sides while the sheets are still passing
	scanner.feed("front")
	until(t, "the first side", func() bool { return len(sidesOf(session)) == 1 })
	if !session.Scanning() {
		t.Error("with sheets still in the feeder the scan is over")
	}
	scanner.feed("the back")
	scanner.feed("3")
	scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })

	state, failure, _ := session.Snapshot()
	if state != StateDone || failure != nil {
		t.Errorf("at the end: %s, %v", state, failure)
	}
	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:5", "2:8", "3:1"}) {
		t.Errorf("sides %q", sides)
	}
	if left := files(t, session); !reflect.DeepEqual(left, []string{"1.jpg", "2.jpg", "3.jpg"}) {
		t.Errorf("in the folder: %q", left)
	}
	if content, _ := os.ReadFile(session.Path("2")); string(content) != "the back" {
		t.Errorf("side 2 holds %q", content)
	}
	for _, id := range []string{"", "0", "4", "2.jpg", "../2", "scan-1-side-2.tmp"} {
		if path := session.Path(id); path != "" {
			t.Errorf("Path(%q) = %s", id, path)
		}
	}
}

func TestOneScanAtATime(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	if !session.Start(Before) {
		t.Fatal("the scan does not start")
	}
	if session.Start(Before) {
		t.Error("a second scan starts under the first")
	}
	scanner.feed("side")
	scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })
	if _, scans := scanner.count(); scans != 1 {
		t.Errorf("the scanner was asked for %d scans", scans)
	}
}

// A second scan goes after the sides already there: the sheets of a thick document in two rounds.
func TestSecondScanGoesAfterTheFirst(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, nil, "a", "bb")
	scanAll(t, session, scanner, nil, "ccc")

	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:1", "2:2", "3:3"}) {
		t.Errorf("sides %q", sides)
	}
	if state, _, _ := session.Snapshot(); state != StateDone {
		t.Errorf("state %s", state)
	}
}

func TestScanThatStopsKeepsTheSidesAndSaysWhy(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, &ScanError{Code: ErrorJam, Message: "a sheet is stuck"}, "a", "bb")

	state, failure, _ := session.Snapshot()
	if state != StateError || failure == nil || failure.Code != ErrorJam || failure.Message != "a sheet is stuck" {
		t.Errorf("%s, %v", state, failure)
	}
	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:1", "2:2"}) {
		t.Errorf("sides %q", sides)
	}

	// the sheet is freed and the scan started again: the error goes away, the sides stay
	if !session.Start(Before) {
		t.Fatal("after an error the scan does not start")
	}
	if state, failure, _ := session.Snapshot(); state != StateScanning || failure != nil {
		t.Errorf("started again: %s, %v", state, failure)
	}
	scanner.feed("ccc")
	scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })
	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:1", "2:2", "3:3"}) {
		t.Errorf("sides %q", sides)
	}
}

func TestScanOfAnEmptyFeeder(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, &ScanError{Code: ErrorEmpty})

	state, failure, pages := session.Snapshot()
	if state != StateError || failure == nil || failure.Code != ErrorEmpty || len(pages) != 0 {
		t.Errorf("%s, %v, %d sides", state, failure, len(pages))
	}
}

// A file the driver announces and is not there is left out, without stopping the scan.
func TestSideWithoutItsFileIsLeftOut(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, nil, "a")
	session.add(filepath.Join(session.dir, "missing.tmp"))
	scanAll(t, session, scanner, nil, "ccc")

	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:1", "3:3"}) {
		t.Errorf("sides %q", sides)
	}
}

func TestClearDeletesEverySide(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, &ScanError{Code: ErrorJam}, "a", "bb")

	if !session.Clear() {
		t.Fatal("Clear refused after the scan")
	}
	state, failure, pages := session.Snapshot()
	if state != StateIdle || failure != nil || len(pages) != 0 {
		t.Errorf("%s, %v, %d sides", state, failure, len(pages))
	}
	if left := files(t, session); len(left) != 0 {
		t.Errorf("still in the folder: %q", left)
	}
	if path := session.Path("1"); path != "" {
		t.Errorf("side 1 is still %s", path)
	}

	// the ids go on: the page tells a side by its id, and must not take a new one for one it had
	scanAll(t, session, scanner, nil, "ccc")
	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"3:3"}) {
		t.Errorf("after Clear, sides %q", sides)
	}
}

func TestClearIsRefusedDuringAScan(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	if !session.Start(Before) {
		t.Fatal("the scan does not start")
	}
	scanner.feed("a")
	until(t, "the first side", func() bool { return len(sidesOf(session)) == 1 })

	if session.Clear() {
		t.Error("Clear went through under a scan")
	}
	if sides := sidesOf(session); !reflect.DeepEqual(sides, []string{"1:1"}) {
		t.Errorf("sides %q", sides)
	}
	scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })
	if !session.Clear() {
		t.Error("Clear refused after the scan")
	}
}

// When the agent quits no scan is left on the computer.
func TestCloseLeavesNothingOnTheComputer(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	scanAll(t, session, scanner, nil, "a", "bb")

	session.Close()

	if _, err := os.Stat(session.dir); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
	if _, _, pages := session.Snapshot(); len(pages) != 0 || session.Path("1") != "" {
		t.Errorf("%d sides still listed", len(pages))
	}
}

func TestA4InDots(t *testing.T) {
	for resolution, want := range map[int][2]int{150: {1240, 1754}, 200: {1654, 2339}, 300: {2480, 3508}} {
		if width, height := A4(resolution); width != want[0] || height != want[1] {
			t.Errorf("at %d dots: %d x %d, want %d x %d", resolution, width, height, want[0], want[1])
		}
	}
}

func TestSettingsAmongWhatTheScannerHas(t *testing.T) {
	has := &Capabilities{Source: SourceFeeder, Sources: []string{SourceFeeder}, Resolutions: []int{150, 300},
		Colours: []string{ColourColour, ColourGrey}}
	good := Settings{Scanner: "one", Source: SourceFeeder, Resolution: 150, Colour: ColourGrey}
	if !has.Has(good) {
		t.Errorf("%+v is not among %+v", good, has)
	}
	for name, change := range map[string]func(*Settings){
		"a source it does not have":     func(s *Settings) { s.Source = SourceFlatbed },
		"a resolution it does not have": func(s *Settings) { s.Resolution = 200 },
		"a colour it does not have":     func(s *Settings) { s.Colour = "blue" },
	} {
		settings := good
		change(&settings)
		if has.Has(settings) {
			t.Errorf("%s: %+v counts as among %+v", name, settings, has)
		}
	}
}

// The page of before sends no settings: it gets the feeder, 200 dots and colour, as always.
func TestScanOfThePageOfBefore(t *testing.T) {
	if want := (Settings{Source: SourceFeeder, Resolution: 200, Colour: ColourColour}); Before != want {
		t.Errorf("the page of before gets %+v, want %+v", Before, want)
	}
}

func TestScanHandsTheSettingsToTheScanner(t *testing.T) {
	scanner := newFeeder()
	session := sessionWith(t, scanner)
	asked := Settings{Scanner: "one", Source: SourceFlatbed, Resolution: 300, Colour: ColourGrey}
	if !session.Start(asked) {
		t.Fatal("the scan does not start")
	}
	scanner.feed("a side")
	scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !session.Scanning() })
	scanner.mu.Lock()
	defer scanner.mu.Unlock()
	if !reflect.DeepEqual(scanner.settings, []Settings{asked}) {
		t.Errorf("the scanner got %+v, want %+v", scanner.settings, asked)
	}
}
