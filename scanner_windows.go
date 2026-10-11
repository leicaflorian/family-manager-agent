package main

import (
	"errors"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/image/bmp"
)

// The scanner on Windows, through WIA (the driver Windows itself uses, the one of "Windows Scan").
// It is the automation side of WIA (WIA.DeviceManager): the objects a script would use.

// The properties of WIA used here, by their number.
const (
	wiaName         = "7"    // of a device in the list: the name Windows shows
	wiaPnP          = "16"   // of a device in the list: how Windows reaches it
	wiaSheetWidth   = "3076" // of the device: the widest sheet of the feeder, in thousandths of an inch
	wiaSheetHeight  = "3077"
	wiaSheetPlace   = "3078" // of the device: where the feeder keeps the sheet (0 left, 1 centre, 2 right)
	wiaCapabilities = "3086" // of the device: what it has (feeder, both sides)
	wiaStatus       = "3087" // of the device: whether the feeder holds sheets
	wiaSelect       = "3088" // of the device: where to take the sheets from
	wiaPages        = "3096" // of the device: how many sides for each request
	wiaPageSize     = "3097" // the size of the sheet: 0 is A4
	wiaIntent       = "6146" // of the item: 1 is colour, 2 is grey
	wiaHorizontal   = "6147" // dots per inch
	wiaVertical     = "6148"
	wiaLeft         = "6149" // the area to scan, in dots
	wiaTop          = "6150"
	wiaWidth        = "6151"
	wiaHeight       = "6152"
)

const (
	wiaScanner    = 1 // DeviceInfo.Type of a scanner
	wiaFeeder     = 1 // in the capabilities and in the choice: the feeder
	wiaFlatbed    = 2 // in the capabilities and in the choice: the glass
	wiaDuplex     = 4 // in the capabilities and in the choice: both sides
	wiaFeedReady  = 1 // in the status: there are sheets in the feeder
	wiaColour     = 1
	wiaGrey       = 2
	wiaFormatJPEG = "{B96B3CAE-0728-11D3-9D7B-0000F81EF32E}"
	jpegQuality   = 85
)

// The answers of WIA that are not a fault of the program.
const (
	wiaPaperJam      = 0x80210002
	wiaPaperEmpty    = 0x80210003
	wiaPaperProblem  = 0x80210004
	wiaOffline       = 0x80210005
	wiaCommunication = 0x8021000A
	wiaCoverOpen     = 0x80210016
)

// How old the answer of WIA about the scanner may be before Status asks again, and from how long
// an answer of WIA is written in the log as slow.
const (
	statusAge = 3 * time.Second
	slowWIA   = 2 * time.Second
)

// usual are the resolutions offered of a driver that takes any of a range, or does not say which.
var usual = []int{75, 100, 150, 200, 300, 400, 600, 1200}

// windowsDriver sends every call to WIA from one thread: the objects of WIA belong to the thread
// that created them.
type windowsDriver struct {
	calls chan func()

	// What WIA last said of the scanner asked. Asking it can take longer than the page waits for
	// an answer (on 11/10/2026 the page gave up after its 12 seconds), so Status answers with
	// this and has WIA asked again behind it.
	mu       sync.Mutex
	chosen   bool   // a page asked for a scanner
	scanner  string // the one last asked, and set on what: "" is the page of before
	source   string
	state    string
	has      *Capabilities
	asked    time.Time     // when WIA last answered
	asking   bool          // a question to WIA is on its way
	answered chan struct{} // closed at the first answer of WIA about the scanner now asked
}

func newDriver() Driver {
	driver := &windowsDriver{calls: make(chan func()), state: ScannerOff, answered: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
			logf("COM: %v", err)
		}
		for call := range driver.calls {
			call()
		}
	}()
	return driver
}

// do runs a call on the thread of WIA and waits. A failure of WIA inside it comes back as an error.
func (d *windowsDriver) do(call func()) (err error) {
	done := make(chan struct{})
	d.calls <- func() {
		defer close(done)
		defer func() {
			if failure := recover(); failure != nil {
				if known, ok := failure.(error); ok {
					err = known
				} else {
					err = fmt.Errorf("%v", failure)
				}
			}
		}()
		call()
	}
	<-done
	return err
}

// List says the scanners Windows lists, the ones of the network too, without connecting to any.
func (d *windowsDriver) List() []Scanner {
	list := []Scanner{}
	err := d.do(func() {
		devices(func(info *ole.IDispatch) bool {
			if get(info, "Type").Val != wiaScanner {
				return false
			}
			found := Scanner{ID: get(info, "DeviceID").ToString(), Name: words(info, wiaName), Connection: ConnectionUSB}
			if found.Name == "" {
				found.Name = found.ID
			}
			if onNetwork(info) {
				found.Connection = ConnectionNetwork
			}
			list = append(list, found)
			return false
		})
	})
	if err != nil {
		logf("scanners: %v", err)
	}
	return list
}

// Status answers with what WIA last said of the scanner asked, and has it asked again when that
// is old. The page that chooses reads "opening" until WIA has answered about that scanner. The
// page of before waits for the first answer, not longer than openingTime: after that the scanner
// counts as off until WIA says otherwise. The probe waits as long as it takes.
func (d *windowsDriver) Status(scanner, source string) (string, *Capabilities) {
	d.mu.Lock()
	if !d.chosen || scanner != d.scanner || source != d.source {
		d.chosen, d.scanner, d.source = true, scanner, source
		d.state, d.has, d.asked = ScannerOpening, nil, time.Time{}
		d.answered = make(chan struct{})
	}
	if !d.asking && time.Since(d.asked) > statusAge {
		d.asking = true
		go d.refresh(scanner, source)
	}
	answered := d.answered
	d.mu.Unlock()

	switch {
	case probing:
		<-answered
	case scanner == "":
		select {
		case <-answered:
		case <-time.After(openingTime):
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if scanner != d.scanner || source != d.source {
		// another page asked for another scanner in the meantime
		return ScannerOff, nil
	}
	if scanner == "" {
		if d.state == ScannerOpening {
			return ScannerOff, nil
		}
		return d.state, nil
	}
	return d.state, d.has
}

// refresh asks WIA how the scanner asked is, connecting only to that one, and keeps the answer
// for Status.
func (d *windowsDriver) refresh(scanner, source string) {
	started := time.Now()
	status := ScannerOff
	var has *Capabilities
	err := d.do(func() {
		device, _ := connect(scanner)
		if device == nil {
			return
		}
		defer device.Release()
		status, has = look(device, source)
	})
	if err != nil {
		logf("status: %v", err)
	}
	// the time spent waiting for the thread of WIA counts too: it is what the page waited
	if took := time.Since(started); took > slowWIA {
		logf("status: WIA answered %q after %s", status, took.Round(100*time.Millisecond))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.asking = false
	if scanner != d.scanner || source != d.source {
		return
	}
	d.state, d.has, d.asked = status, has, time.Now()
	select {
	case <-d.answered:
	default:
		close(d.answered)
	}
}

func (d *windowsDriver) Scan(settings Settings, dir string, page func(path string)) *ScanError {
	// the feeder is no longer as WIA said: the next Status has it asked again
	defer func() {
		d.mu.Lock()
		d.asked = time.Time{}
		d.mu.Unlock()
	}()
	// between the request and the first side WIA may stay silent for good: every step leaves a
	// line, and the last one says where the scan stopped
	logf("scan: asked")
	asked := time.Now()
	var result *ScanError
	err := d.do(func() {
		if waited := time.Since(asked); waited > slowWIA {
			logf("scan: waited %s for the thread of WIA", waited.Round(100*time.Millisecond))
		}
		result = scan(settings, dir, page)
	})
	if err != nil {
		return scanFailure(err, 0)
	}
	return result
}

// Describe lists what Windows sees without connecting to any scanner, and connects to the one
// asked to say every property it has.
func (d *windowsDriver) Describe(scanner string) string {
	var text strings.Builder
	err := d.do(func() { describe(&text, scanner) })
	if err != nil {
		fmt.Fprintf(&text, "\nWIA stopped: %v\n", err)
	}
	return text.String()
}

func (d *windowsDriver) Close() {}

// settle is for the probe on the Mac: here WIA answers when asked.
func settle(wait time.Duration) {}

// --- the calls to WIA: they panic with the error of WIA, caught by do ---

func get(object *ole.IDispatch, name string, arguments ...any) *ole.VARIANT {
	value, err := oleutil.GetProperty(object, name, arguments...)
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	return value
}

// read is a value for the log: a failure is not a reason to stop.
func read(object *ole.IDispatch, name string) any {
	value, err := oleutil.GetProperty(object, name)
	if err != nil {
		return "?"
	}
	return value.Value()
}

func call(object *ole.IDispatch, name string, arguments ...any) (*ole.VARIANT, error) {
	return oleutil.CallMethod(object, name, arguments...)
}

// property is one property of a device or of an item, found by its number.
func property(object *ole.IDispatch, id string) *ole.IDispatch {
	properties := get(object, "Properties").ToIDispatch()
	defer properties.Release()
	exists, err := oleutil.CallMethod(properties, "Exists", id)
	if err != nil || exists.Value() != true {
		return nil
	}
	return get(properties, "Item", id).ToIDispatch()
}

func number(object *ole.IDispatch, id string) int64 {
	found := property(object, id)
	if found == nil {
		return 0
	}
	defer found.Release()
	switch value := get(found, "Value").Value().(type) {
	case int32:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case uint32:
		return int64(value)
	}
	return 0
}

// put writes a property, and when the driver does not take the value leaves in the log why, the
// value that stays and the ones it takes.
func put(object *ole.IDispatch, id string, value int32) error {
	found := property(object, id)
	if found == nil {
		return fmt.Errorf("the scanner has no property %s", id)
	}
	defer found.Release()
	if _, err := oleutil.PutProperty(found, "Value", value); err != nil {
		logf("WIA property %s = %d refused: %v (0x%08X); it is %v, read only %v, takes %s",
			id, value, err, codeOf(err), read(found, "Value"), read(found, "IsReadOnly"), allowed(found))
		return fmt.Errorf("%v (0x%08X)", err, codeOf(err))
	}
	return nil
}

// set writes a property and says whether the driver took it: a scanner that does not have one, or
// refuses a value, is not a reason to stop.
func set(object *ole.IDispatch, id string, value int32) bool {
	return put(object, id, value) == nil
}

// whole is a number read from WIA, whatever its type.
func whole(value any) (int, bool) {
	switch number := value.(type) {
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case int:
		return number, true
	case uint32:
		return int(number), true
	case int16:
		return int(number), true
	case uint16:
		return int(number), true
	}
	return 0, false
}

// resolutions says the dots per inch the driver takes, read from the first item of the scanner:
// its list, or the usual ones that fit its range. A driver that does not say gets the usual ones,
// and refuses at the scan the one it does not have.
func resolutions(device *ole.IDispatch) []int {
	list := []int{}
	items := get(device, "Items").ToIDispatch()
	defer items.Release()
	if get(items, "Count").Val < 1 {
		return list
	}
	item := get(items, "Item", int32(1)).ToIDispatch()
	defer item.Release()
	found := property(item, wiaHorizontal)
	if found == nil {
		return list
	}
	defer found.Release()

	switch fmt.Sprint(read(found, "SubType")) {
	case "1": // a range
		least, _ := whole(read(found, "SubTypeMin"))
		most, _ := whole(read(found, "SubTypeMax"))
		step, _ := whole(read(found, "SubTypeStep"))
		if step < 1 {
			step = 1
		}
		for _, dots := range usual {
			if dots >= least && dots <= most && (dots-least)%step == 0 {
				list = append(list, dots)
			}
		}
		return list
	case "2", "3": // a list, or flags
		values, err := oleutil.GetProperty(found, "SubTypeValues")
		if err != nil || values.VT != ole.VT_DISPATCH || values.Val == 0 {
			break
		}
		vector := values.ToIDispatch()
		defer vector.Release()
		count, _ := whole(read(vector, "Count"))
		for index := 1; index <= count; index++ {
			entry, err := oleutil.GetProperty(vector, "Item", int32(index))
			if err != nil {
				break
			}
			if dots, ok := whole(entry.Value()); ok {
				list = append(list, dots)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	return append(list, usual...)
}

// look says how the scanner connected is and what it has: where it takes the sheets from, set on
// the source asked when it has it, and the resolutions.
func look(device *ole.IDispatch, source string) (string, *Capabilities) {
	capabilities := number(device, wiaCapabilities)
	has := &Capabilities{Sources: []string{}, Colours: []string{ColourColour, ColourGrey}}
	if capabilities&wiaFeeder != 0 {
		has.Sources = append(has.Sources, SourceFeeder)
	}
	// a scanner that says nothing of a feeder has a glass
	if capabilities&wiaFlatbed != 0 || capabilities&wiaFeeder == 0 {
		has.Sources = append(has.Sources, SourceFlatbed)
	}
	has.Source = has.Sources[0]
	for _, there := range has.Sources {
		if there == source {
			has.Source = source
		}
	}
	has.Resolutions = resolutions(device)

	// the glass does not say whether a sheet is on it
	state := ScannerReady
	if has.Source == SourceFeeder && number(device, wiaStatus)&wiaFeedReady == 0 {
		state = ScannerEmpty
	}
	return state, has
}

// allowed says which values the driver takes for a property: for the log of a value refused.
func allowed(found *ole.IDispatch) string {
	switch fmt.Sprint(read(found, "SubType")) {
	case "1": // a range
		return fmt.Sprintf("from %v to %v by %v", read(found, "SubTypeMin"), read(found, "SubTypeMax"), read(found, "SubTypeStep"))
	case "2", "3": // a list, or flags
		values, err := oleutil.GetProperty(found, "SubTypeValues")
		if err != nil || values.VT != ole.VT_DISPATCH || values.Val == 0 {
			return "?"
		}
		vector := values.ToIDispatch()
		defer vector.Release()
		count, _ := read(vector, "Count").(int32)
		var list []string
		for index := int32(1); index <= count; index++ {
			entry, err := oleutil.GetProperty(vector, "Item", index)
			if err != nil {
				break
			}
			list = append(list, fmt.Sprint(entry.Value()))
		}
		return strings.Join(list, " ")
	}
	return "anything"
}

// devices calls each with every scanner Windows sees, until one answers true.
func devices(each func(info *ole.IDispatch) bool) {
	unknown, err := oleutil.CreateObject("WIA.DeviceManager")
	if err != nil {
		panic(fmt.Errorf("WIA.DeviceManager: %w", err))
	}
	defer unknown.Release()
	manager, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		panic(err)
	}
	defer manager.Release()
	infos := get(manager, "DeviceInfos").ToIDispatch()
	defer infos.Release()

	count := int(get(infos, "Count").Val)
	for index := 1; index <= count; index++ {
		info := get(infos, "Item", int32(index)).ToIDispatch()
		stop := each(info)
		info.Release()
		if stop {
			return
		}
	}
}

// onNetwork is true for a scanner Windows found on the network (WSD, eSCL), which it lists as a
// software device, and not on a cable. The agent leaves those alone, as on the Mac: connecting
// wakes them up, and waits for as long as they do not answer. On 11/10/2026 a PC listed a printer
// on the Wi-Fi twice before the scanner, and every question about the status went through it.
func onNetwork(info *ole.IDispatch) bool {
	return strings.HasPrefix(strings.ToLower(words(info, wiaPnP)), `\\?\swd#`)
}

// words is a property that holds a text, or "" when the object does not have it.
func words(object *ole.IDispatch, id string) string {
	found := property(object, id)
	if found == nil {
		return ""
	}
	defer found.Release()
	return fmt.Sprint(read(found, "Value"))
}

// The last scanner asked by its id that did not connect, and why: written in the log once, not
// at every question of the page.
var unreached string

// connect connects to the scanner asked by its id, on the cable or on the network, and to no
// other; with "", the page of before, to the first on the cable with a feeder. It is nil when the
// scanner is not there or does not answer. The caller releases it. The second answer is the id
// Windows gives that scanner, for the log of a scan.
func connect(scanner string) (*ole.IDispatch, string) {
	if scanner == "" {
		return feederDevice()
	}
	var found *ole.IDispatch
	failure := scanner + ": not in the list of Windows"
	devices(func(info *ole.IDispatch) bool {
		if get(info, "Type").Val != wiaScanner || get(info, "DeviceID").ToString() != scanner {
			return false
		}
		started := time.Now()
		connected, err := call(info, "Connect")
		if took := time.Since(started); took > slowWIA {
			logf("WIA: connecting to %s took %s", scanner, took.Round(100*time.Millisecond))
		}
		if err != nil {
			failure = fmt.Sprintf("%s: %v (0x%08X)", scanner, err, codeOf(err))
			return true
		}
		found, failure = connected.ToIDispatch(), ""
		return true
	})
	if failure != "" && failure != unreached {
		logf("WIA: no connection to %s", failure)
	}
	unreached = failure
	return found, scanner
}

// feederDevice connects to the first scanner on the cable with a feeder, or is nil: for the page
// of before, which chooses no scanner. The caller releases it. The second answer is the id
// Windows gives that scanner, for the log of a scan.
func feederDevice() (*ole.IDispatch, string) {
	var found *ole.IDispatch
	var id string
	devices(func(info *ole.IDispatch) bool {
		if get(info, "Type").Val != wiaScanner || onNetwork(info) {
			return false
		}
		started := time.Now()
		connected, err := call(info, "Connect")
		if took := time.Since(started); took > slowWIA {
			logf("WIA: connecting to %s took %s", get(info, "DeviceID").ToString(), took.Round(100*time.Millisecond))
		}
		if err != nil {
			return false
		}
		device := connected.ToIDispatch()
		if number(device, wiaCapabilities)&wiaFeeder == 0 {
			device.Release()
			return false
		}
		found, id = device, get(info, "DeviceID").ToString()
		return true
	})
	return found, id
}

func scan(settings Settings, dir string, page func(path string)) *ScanError {
	started := time.Now()
	// the page of before sends nothing: the first scanner with a feeder, and the resolution the
	// driver keeps when it refuses the one asked
	before := settings.Scanner == ""
	glass := settings.Source == SourceFlatbed
	device, id := connect(settings.Scanner)
	if device == nil {
		if before {
			logf("scan: no scanner with a feeder")
		} else {
			logf("scan: %s is not there", settings.Scanner)
		}
		return &ScanError{Code: ErrorOff}
	}
	defer device.Release()
	capabilities := number(device, wiaCapabilities)
	logf("scan: %s connected in %s, capabilities %d, status %d, asked %s, %d dots, %s", id,
		time.Since(started).Round(100*time.Millisecond), capabilities, number(device, wiaStatus),
		settings.Source, settings.Resolution, settings.Colour)

	choice := int32(wiaFeeder)
	if glass {
		choice = wiaFlatbed
	} else if capabilities&wiaDuplex != 0 {
		choice |= wiaDuplex
	}
	set(device, wiaSelect, choice)
	set(device, wiaPages, 1)
	set(device, wiaPageSize, 0)

	items := get(device, "Items").ToIDispatch()
	defer items.Release()
	logf("scan: items %d", get(items, "Count").Val)
	item := get(items, "Item", int32(1)).ToIDispatch()
	defer item.Release()

	// the colour first: some drivers put the resolution back when it changes
	intent := int32(wiaColour)
	if settings.Colour == ColourGrey {
		intent = wiaGrey
	}
	set(item, wiaIntent, intent)
	refused := put(item, wiaHorizontal, int32(settings.Resolution))
	if err := put(item, wiaVertical, int32(settings.Resolution)); refused == nil {
		refused = err
	}
	// the page that chooses asked for a resolution of this scanner: one the driver refuses stops
	// the scan, it is not replaced with another
	if refused != nil && !before {
		return &ScanError{Code: ErrorScanner, Message: fmt.Sprintf("the scanner refused %d dots: %v", settings.Resolution, refused)}
	}
	set(item, wiaPageSize, 0)
	// the area is in dots: it is an A4 only at the resolution the driver kept, which is not always
	// the one asked. On 11/10/2026 a PC gave the middle two thirds of the sheet, as the area of
	// 200 dots does at 300.
	across, down := int(number(item, wiaHorizontal)), int(number(item, wiaVertical))
	if across <= 0 {
		across = settings.Resolution
	}
	if down <= 0 {
		down = settings.Resolution
	}
	width, _ := A4(across)
	_, height := A4(down)
	set(item, wiaLeft, 0)
	set(item, wiaTop, 0)
	set(item, wiaWidth, int32(width))
	set(item, wiaHeight, int32(height))

	// what the driver kept of what was asked, and how its feeder holds the sheet: an area in dots
	// is the A4 only at the resolution asked, and from the side where the sheet lies
	logf("scan: feeder choice %d, sides for each request %d, intent %d, %d x %d dots, area %d x %d from %d,%d, page size %d",
		number(device, wiaSelect), number(device, wiaPages), number(item, wiaIntent), number(item, wiaHorizontal),
		number(item, wiaVertical), number(item, wiaWidth), number(item, wiaHeight), number(item, wiaLeft),
		number(item, wiaTop), number(item, wiaPageSize))
	logf("scan: feeder sheet %d x %d thousandths of an inch, placed %d: asking for the first side",
		number(device, wiaSheetWidth), number(device, wiaSheetHeight), number(device, wiaSheetPlace))

	// one side for each request, until WIA says the sheets are over
	for side := 1; ; side++ {
		asked := time.Now()
		transferred, err := call(item, "Transfer", wiaFormatJPEG)
		if err != nil {
			failure := scanFailure(err, side-1)
			if failure == nil {
				logf("scan: over after %d sides", side-1)
			}
			return failure
		}
		image := transferred.ToIDispatch()
		// what arrived, which is not always what was asked
		size := fmt.Sprintf("%v x %v at %v dots", read(image, "Width"), read(image, "Height"), read(image, "HorizontalResolution"))
		path, err := saveJPEG(image, dir, side)
		image.Release()
		if err != nil {
			return &ScanError{Code: ErrorScanner, Message: err.Error()}
		}
		page(path)
		logf("scan: side %d in %s, %s", side, time.Since(asked).Round(100*time.Millisecond), size)
		if glass {
			// on the glass there is one sheet, and one side of it
			logf("scan: over, one side from the glass")
			return nil
		}
	}
}

// saveJPEG writes a side as a JPEG. A driver may hand a BMP even when asked for a JPEG: it is converted.
func saveJPEG(image *ole.IDispatch, dir string, side int) (string, error) {
	raw := filepath.Join(dir, fmt.Sprintf("wia-%d.tmp", side))
	target := filepath.Join(dir, fmt.Sprintf("wia-%d.jpg", side))
	os.Remove(raw)
	if _, err := call(image, "SaveFile", raw); err != nil {
		return "", fmt.Errorf("SaveFile: %w", err)
	}
	if strings.EqualFold(get(image, "FormatID").ToString(), wiaFormatJPEG) {
		return target, os.Rename(raw, target)
	}
	defer os.Remove(raw)
	source, err := os.Open(raw)
	if err != nil {
		return "", err
	}
	defer source.Close()
	decoded, err := bmp.Decode(source)
	if err != nil {
		return "", fmt.Errorf("the scanner sent an image that is neither JPEG nor BMP: %w", err)
	}
	file, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return target, jpeg.Encode(file, decoded, &jpeg.Options{Quality: jpegQuality})
}

// codeOf is the number WIA gave with an error, or 0.
func codeOf(err error) uint32 {
	var failure *ole.OleError
	if !errors.As(err, &failure) {
		return 0
	}
	if detail, ok := failure.SubError().(ole.EXCEPINFO); ok && detail.SCODE() != 0 {
		return detail.SCODE()
	}
	return uint32(failure.Code())
}

// scanFailure turns an error of WIA into one of our codes. "No more sheets" after at least one
// side is the normal end of a scan.
func scanFailure(err error, sides int) *ScanError {
	code := codeOf(err)
	message := fmt.Sprintf("%v (0x%08X)", err, code)
	switch code {
	case wiaPaperEmpty:
		if sides > 0 {
			return nil
		}
		return &ScanError{Code: ErrorEmpty}
	case wiaPaperJam, wiaPaperProblem, wiaCoverOpen:
		return &ScanError{Code: ErrorJam, Message: message}
	case wiaOffline, wiaCommunication:
		return &ScanError{Code: ErrorOff, Message: message}
	}
	return &ScanError{Code: ErrorScanner, Message: message}
}

// describe lists what Windows sees, and of the scanner asked every property: for the probe and
// the log. It connects only to that one.
func describe(text *strings.Builder, scanner string) {
	seen := 0
	devices(func(info *ole.IDispatch) bool {
		seen++
		kind := get(info, "Type").Val
		fmt.Fprintf(text, "- device of type %d (1 is a scanner), id %s\n", kind, get(info, "DeviceID").ToString())
		properties(text, info, "  ")
		if kind != wiaScanner {
			return false
		}
		if onNetwork(info) {
			fmt.Fprintln(text, "  on the network")
		}
		if scanner == "" || get(info, "DeviceID").ToString() != scanner {
			return false
		}
		connected, err := call(info, "Connect")
		if err != nil {
			fmt.Fprintf(text, "  does not connect: %v (0x%08X)\n", err, codeOf(err))
			return false
		}
		device := connected.ToIDispatch()
		defer device.Release()
		capabilities := number(device, wiaCapabilities)
		fmt.Fprintf(text, "  feeder: %v, glass: %v, both sides: %v, sheets in the feeder: %v\n",
			capabilities&wiaFeeder != 0, capabilities&wiaFlatbed != 0, capabilities&wiaDuplex != 0,
			number(device, wiaStatus)&wiaFeedReady != 0)
		fmt.Fprintln(text, "  properties of the device:")
		properties(text, device, "    ")

		items := get(device, "Items").ToIDispatch()
		defer items.Release()
		count := int(get(items, "Count").Val)
		fmt.Fprintf(text, "  items: %d\n", count)
		for index := 1; index <= count; index++ {
			item := get(items, "Item", int32(index)).ToIDispatch()
			fmt.Fprintf(text, "  properties of item %d:\n", index)
			properties(text, item, "    ")
			item.Release()
		}
		return false
	})
	fmt.Fprintf(text, "devices seen by WIA: %d\n", seen)
}

func properties(text *strings.Builder, object *ole.IDispatch, indent string) {
	list := get(object, "Properties").ToIDispatch()
	defer list.Release()
	count := int(get(list, "Count").Val)
	for index := 1; index <= count; index++ {
		entry := get(list, "Item", int32(index)).ToIDispatch()
		value := "?"
		if read, err := oleutil.GetProperty(entry, "Value"); err == nil {
			value = fmt.Sprint(read.Value())
		}
		// the values the driver takes, when it says them: of the resolution (6147) the page offers those
		takes := ""
		if values := allowed(entry); values != "anything" {
			takes = ", takes " + values
		}
		fmt.Fprintf(text, "%s%d %s = %s%s\n", indent, get(entry, "PropertyID").Val, get(entry, "Name").ToString(), value, takes)
		entry.Release()
	}
}
