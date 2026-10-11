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
	wiaCapabilities = "3086" // of the device: what it has (feeder, both sides)
	wiaStatus       = "3087" // of the device: whether the feeder holds sheets
	wiaSelect       = "3088" // of the device: where to take the sheets from
	wiaPages        = "3096" // of the device: how many sides for each request
	wiaPageSize     = "3097" // the size of the sheet: 0 is A4
	wiaIntent       = "6146" // of the item: 1 is colour
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
	wiaDuplex     = 4 // in the capabilities and in the choice: both sides
	wiaFeedReady  = 1 // in the status: there are sheets in the feeder
	wiaColour     = 1
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

// windowsDriver sends every call to WIA from one thread: the objects of WIA belong to the thread
// that created them.
type windowsDriver struct {
	calls chan func()

	// What WIA last said of the scanner. Asking it can take longer than the page waits for an
	// answer (on 11/10/2026 the page gave up after its 12 seconds), so Status answers with this
	// and has WIA asked again behind it.
	mu     sync.Mutex
	state  string
	asked  time.Time     // when WIA last answered
	asking bool          // a question to WIA is on its way
	first  chan struct{} // closed at the first answer of WIA
	once   sync.Once
}

func newDriver() Driver {
	driver := &windowsDriver{calls: make(chan func()), state: ScannerOff, first: make(chan struct{})}
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

// Status answers with what WIA last said, and has it asked again when that is old. It waits only
// for the first answer, and not longer than openingTime: after that the scanner counts as off
// until WIA says otherwise. The probe waits as long as it takes.
func (d *windowsDriver) Status() string {
	d.mu.Lock()
	if !d.asking && time.Since(d.asked) > statusAge {
		d.asking = true
		go d.refresh()
	}
	d.mu.Unlock()

	if probing {
		<-d.first
	} else {
		select {
		case <-d.first:
		case <-time.After(openingTime):
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// refresh asks WIA how the scanner is and keeps the answer for Status.
func (d *windowsDriver) refresh() {
	started := time.Now()
	status := ScannerOff
	err := d.do(func() {
		device := feederDevice()
		if device == nil {
			return
		}
		defer device.Release()
		status = ScannerEmpty
		if number(device, wiaStatus)&wiaFeedReady != 0 {
			status = ScannerReady
		}
	})
	if err != nil {
		logf("status: %v", err)
	}
	// the time spent waiting for the thread of WIA counts too: it is what the page waited
	if took := time.Since(started); took > slowWIA {
		logf("status: WIA answered %q after %s", status, took.Round(100*time.Millisecond))
	}
	d.mu.Lock()
	d.state, d.asked, d.asking = status, time.Now(), false
	d.mu.Unlock()
	d.once.Do(func() { close(d.first) })
}

func (d *windowsDriver) Scan(dir string, page func(path string)) *ScanError {
	// the feeder is no longer as WIA said: the next Status has it asked again
	defer func() {
		d.mu.Lock()
		d.asked = time.Time{}
		d.mu.Unlock()
	}()
	var result *ScanError
	err := d.do(func() { result = scan(dir, page) })
	if err != nil {
		return scanFailure(err, 0)
	}
	return result
}

func (d *windowsDriver) Describe() string {
	var text strings.Builder
	err := d.do(func() { describe(&text) })
	if err != nil {
		fmt.Fprintf(&text, "\nWIA stopped: %v\n", err)
	}
	return text.String()
}

func (d *windowsDriver) Close() {}

func settle() {}

// --- the calls to WIA: they panic with the error of WIA, caught by do ---

func get(object *ole.IDispatch, name string, arguments ...any) *ole.VARIANT {
	value, err := oleutil.GetProperty(object, name, arguments...)
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	return value
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

// set writes a property and says whether the driver took it: a scanner that does not have one, or
// refuses a value, is not a reason to stop.
func set(object *ole.IDispatch, id string, value int32) bool {
	found := property(object, id)
	if found == nil {
		return false
	}
	defer found.Release()
	if _, err := oleutil.PutProperty(found, "Value", value); err != nil {
		logf("WIA property %s = %d refused: %v", id, value, err)
		return false
	}
	return true
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

// feederDevice connects to the first scanner with a feeder, or is nil. The caller releases it.
func feederDevice() *ole.IDispatch {
	var found *ole.IDispatch
	devices(func(info *ole.IDispatch) bool {
		if get(info, "Type").Val != wiaScanner {
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
		found = device
		return true
	})
	return found
}

func scan(dir string, page func(path string)) *ScanError {
	device := feederDevice()
	if device == nil {
		return &ScanError{Code: ErrorOff}
	}
	defer device.Release()

	choice := int32(wiaFeeder)
	if number(device, wiaCapabilities)&wiaDuplex != 0 {
		choice |= wiaDuplex
	}
	set(device, wiaSelect, choice)
	set(device, wiaPages, 1)
	set(device, wiaPageSize, 0)

	items := get(device, "Items").ToIDispatch()
	defer items.Release()
	item := get(items, "Item", int32(1)).ToIDispatch()
	defer item.Release()

	// the colour first: some drivers put the resolution back when it changes
	set(item, wiaIntent, wiaColour)
	set(item, wiaHorizontal, Resolution)
	set(item, wiaVertical, Resolution)
	set(item, wiaPageSize, 0)
	set(item, wiaLeft, 0)
	set(item, wiaTop, 0)
	set(item, wiaWidth, A4Width)
	set(item, wiaHeight, A4Height)

	// one side for each request, until WIA says the sheets are over
	for side := 1; ; side++ {
		transferred, err := call(item, "Transfer", wiaFormatJPEG)
		if err != nil {
			return scanFailure(err, side-1)
		}
		image := transferred.ToIDispatch()
		path, err := saveJPEG(image, dir, side)
		image.Release()
		if err != nil {
			return &ScanError{Code: ErrorScanner, Message: err.Error()}
		}
		page(path)
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

// describe lists what Windows sees, with every property of the scanners: for the probe.
func describe(text *strings.Builder) {
	seen := 0
	devices(func(info *ole.IDispatch) bool {
		seen++
		kind := get(info, "Type").Val
		fmt.Fprintf(text, "- device of type %d (1 is a scanner), id %s\n", kind, get(info, "DeviceID").ToString())
		properties(text, info, "  ")
		if kind != wiaScanner {
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
		fmt.Fprintf(text, "  feeder: %v, both sides: %v, sheets in the feeder: %v\n",
			capabilities&wiaFeeder != 0, capabilities&wiaDuplex != 0, number(device, wiaStatus)&wiaFeedReady != 0)
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
		fmt.Fprintf(text, "%s%d %s = %s\n", indent, get(entry, "PropertyID").Val, get(entry, "Name").ToString(), value)
		entry.Release()
	}
}
