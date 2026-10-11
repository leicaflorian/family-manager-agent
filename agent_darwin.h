// The Objective-C side of the agent on the Mac: the scanner (scanner_darwin.m, Image Capture)
// and the windows (dialog_darwin.m). Called from Go through cgo.
#ifndef FM_AGENT_DARWIN_H
#define FM_AGENT_DARWIN_H

// What fmScannerState answers.
enum { FMScannerOff = 0, FMScannerEmpty = 1, FMScannerReady = 2 };

// How a scan ended, for goScanDone.
enum { FMScanDone = 0, FMScanOff = 1, FMScanEmpty = 2, FMScanJam = 3, FMScanFailed = 4 };

// Starts looking for the scanners attached to this Mac. Callable from any thread.
void fmScannerStart(void);
// Closes the session with the scanner.
void fmScannerStop(void);
int fmScannerState(void);
// Feeds the sheets of the feeder into dir, one JPEG per side. Each side goes to goScanPage and
// the end to goScanDone. Returns FMScanDone when the scan started, or why it could not.
int fmScannerScan(const char *dir, int resolution);
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

#endif
