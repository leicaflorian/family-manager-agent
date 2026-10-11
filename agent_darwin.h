// The Objective-C side of the agent on the Mac: the scanner (scanner_darwin.m, Image Capture)
// and the windows (dialog_darwin.m). Called from Go through cgo.
#ifndef FM_AGENT_DARWIN_H
#define FM_AGENT_DARWIN_H

// What fmScannerState answers. Opening: the scanner asked is there and its session is not open yet.
enum { FMScannerOff = 0, FMScannerEmpty = 1, FMScannerReady = 2, FMScannerOpening = 3 };

// How a scan ended, for goScanDone. Settings: the scanner does not have the resolution asked.
enum { FMScanDone = 0, FMScanOff = 1, FMScanEmpty = 2, FMScanJam = 3, FMScanFailed = 4, FMScanSettings = 5 };

// Where the sheets come from. Any is the feeder when the scanner has one, the glass when it does not.
enum { FMUnitAny = 0, FMUnitFeeder = 1, FMUnitFlatbed = 2 };

// Starts looking for the scanners this Mac sees, the ones of the network too. None is opened.
// Callable from any thread.
void fmScannerStart(void);
// Closes the session with the scanner.
void fmScannerStop(void);
// The scanners seen, one per line: id, connection ("usb" or "network") and name, with a tab
// between. The caller frees it.
char *fmScannerList(void);
// Opens the scanner with this id and sets it on the unit, letting go of the one open before.
// With "" it is the page of before: the first scanner on the cable with a feeder.
void fmScannerChoose(const char *id, int unit);
int fmScannerState(void);
// What the chosen scanner has, once it is open and set, with a tab between: the unit it is set
// on, its units, the resolutions of that unit. "" until then. The caller frees it.
char *fmScannerHas(void);
// Feeds the sheets of the feeder into dir, or reads the one on the glass, one JPEG per side. Each
// side goes to goScanPage and the end to goScanDone. Returns FMScanDone when the scan started,
// or why it could not.
int fmScannerScan(const char *dir, int resolution, int grey);
// Stops a scan that gives no sign of life. goScanDone is not called.
void fmScannerCancel(void);
// What the Mac sees, in plain text. The caller frees it.
char *fmScannerDescribe(void);
// Runs the main run loop for some seconds: for the probe, which has no application around it.
void fmRunLoop(double seconds);

// A question with two buttons: 1 for the first, 0 for the second.
int fmAsk(const char *title, const char *text, const char *yes, const char *no);
// A message with one button.
void fmTell(const char *title, const char *text);
// A message with the buttons given, the first being the one of Return: the place of the one
// clicked, from 0, or -1 when the window was closed without a click. The button at the place
// stay (-1 for none) leaves the window open: a click turns the buttons off and calls goChooseStay.
int fmChoose(const char *title, const char *text, const char **buttons, int count, int stay);
// Turns the buttons of the window of fmChoose on again, and gives this title to the one that stays.
void fmChooseDone(const char *title);
// Asks the window on top to close, without a click: 1 when there was one. Callable from any thread.
int fmCloseWindow(void);

#endif
