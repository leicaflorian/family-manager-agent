// The scanner on the Mac, through Image Capture (ImageCaptureCore): the driver is the one macOS
// already has. Everything here runs on the main thread, where Image Capture sends its answers.
#import <Foundation/Foundation.h>
#import <ImageCaptureCore/ImageCaptureCore.h>
#include <stdlib.h>
#include <string.h>
#include "agent_darwin.h"
#include "_cgo_export.h"

// Runs a block on the main thread and waits for it. A block of the run loop, not of the main
// queue: it runs also while a window of the agent is open.
static void onMain(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
        return;
    }
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, ^{
        block();
        dispatch_semaphore_signal(done);
    });
    CFRunLoopWakeUp(CFRunLoopGetMain());
    dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
}

@interface FMScanner : NSObject <ICDeviceBrowserDelegate, ICScannerDeviceDelegate>
@property(strong) ICDeviceBrowser *browser;
// the scanners seen, in the order of arrival
@property(strong) NSMutableArray<ICScannerDevice *> *found;
// the ones that cannot be used: no feeder, or the session did not open
@property(strong) NSMutableArray<ICScannerDevice *> *discarded;
// the one with the session open
@property(strong) ICScannerDevice *scanner;
// the feeder was chosen and the scanner confirmed it: only then a scan is accepted
@property BOOL feeder;
@property BOOL scanning;
@property NSUInteger sides;
@property(strong) NSMutableArray<NSString *> *events;
@end

@implementation FMScanner

- (instancetype)init {
    self = [super init];
    _found = [NSMutableArray array];
    _discarded = [NSMutableArray array];
    _events = [NSMutableArray array];
    return self;
}

- (void)note:(NSString *)format, ... {
    va_list arguments;
    va_start(arguments, format);
    NSString *line = [[NSString alloc] initWithFormat:format arguments:arguments];
    va_end(arguments);
    if (self.events.count >= 200) [self.events removeObjectAtIndex:0];
    [self.events addObject:line];
}

- (void)start {
    if (self.browser) return;
    self.browser = [[ICDeviceBrowser alloc] init];
    self.browser.delegate = self;
    // only the scanners attached to this Mac: the ones of the network have no feeder to look for,
    // and opening a session would wake them up
    self.browser.browsedDeviceTypeMask = ICDeviceTypeMaskScanner | ICDeviceLocationTypeMaskLocal;
    [self.browser start];
}

- (void)stop {
    if (self.scanning) [self.scanner cancelScan];
    if (self.scanner.hasOpenSession) [self.scanner requestCloseSession];
    [self.browser stop];
    self.browser = nil;
    self.scanner = nil;
    self.feeder = NO;
}

// Opens the first scanner not yet discarded. One at a time: whether it has a feeder is known
// only with the session open.
- (void)openNext {
    if (self.scanner) return;
    for (ICScannerDevice *candidate in self.found) {
        if ([self.discarded containsObject:candidate]) continue;
        self.scanner = candidate;
        self.feeder = NO;
        candidate.delegate = self;
        [self note:@"opening %@", candidate.name];
        [candidate requestOpenSession];
        return;
    }
}

- (void)discard:(ICScannerDevice *)device because:(NSString *)reason {
    [self note:@"%@ left aside: %@", device.name, reason];
    if (![self.discarded containsObject:device]) [self.discarded addObject:device];
    if (self.scanner == device) {
        if (device.hasOpenSession) [device requestCloseSession];
        self.scanner = nil;
        self.feeder = NO;
    }
    [self openNext];
}

- (ICScannerFunctionalUnitDocumentFeeder *)feederUnit {
    if (!self.feeder) return nil;
    ICScannerFunctionalUnit *unit = self.scanner.selectedFunctionalUnit;
    if (unit.type != ICScannerFunctionalUnitTypeDocumentFeeder) return nil;
    return (ICScannerFunctionalUnitDocumentFeeder *)unit;
}

- (int)state {
    ICScannerFunctionalUnitDocumentFeeder *unit = [self feederUnit];
    if (!unit) return FMScannerOff;
    return unit.documentLoaded ? FMScannerReady : FMScannerEmpty;
}

// The resolution asked, or the nearest one above that the scanner has.
- (NSUInteger)resolution:(NSUInteger)wanted of:(ICScannerFunctionalUnit *)unit {
    NSIndexSet *supported = unit.supportedResolutions;
    if ([supported containsIndex:wanted] || supported.count == 0) return wanted;
    NSUInteger above = [supported indexGreaterThanIndex:wanted];
    return above != NSNotFound ? above : supported.lastIndex;
}

- (void)cancel {
    if (!self.scanning) return;
    self.scanning = NO;
    [self note:@"scan cancelled"];
    [self.scanner cancelScan];
}

- (int)scanInto:(NSString *)directory resolution:(NSUInteger)resolution {
    ICScannerFunctionalUnitDocumentFeeder *unit = [self feederUnit];
    if (!unit) return FMScanOff;
    if (self.scanning) return FMScanFailed;

    unit.documentType = ICScannerDocumentTypeA4;
    if (unit.supportsDuplexScanning) unit.duplexScanningEnabled = YES;
    unit.resolution = [self resolution:resolution of:unit];
    unit.pixelDataType = ICScannerPixelDataTypeRGB;
    unit.bitDepth = ICScannerBitDepth8Bits;

    self.scanner.transferMode = ICScannerTransferModeFileBased;
    self.scanner.downloadsDirectory = [NSURL fileURLWithPath:directory isDirectory:YES];
    self.scanner.documentName = @"side";
    self.scanner.documentUTI = @"public.jpeg";

    self.scanning = YES;
    self.sides = 0;
    [self note:@"scan asked: %lu dots, both sides %d, sheets %d", (unsigned long)unit.resolution,
        unit.duplexScanningEnabled, unit.documentLoaded];
    [self.scanner requestScan];
    return FMScanDone;
}

- (void)finish:(int)code message:(NSString *)message {
    if (!self.scanning) return;
    self.scanning = NO;
    [self note:@"scan ended: code %d, %lu sides, %@", code, (unsigned long)self.sides, message ?: @"-"];
    goScanDone(code, (char *)(message ?: @"").UTF8String);
}

// What the driver said, turned into one of our codes. An error with no sheets in the feeder and
// no side arrived is an empty feeder; the words of the driver decide between a jam and the rest.
- (int)codeOf:(NSError *)error {
    ICScannerFunctionalUnitDocumentFeeder *unit = [self feederUnit];
    NSString *text = error.localizedDescription.lowercaseString ?: @"";
    if ([text containsString:@"jam"] || [text containsString:@"incepp"]) return FMScanJam;
    if (unit && !unit.documentLoaded && self.sides == 0) return FMScanEmpty;
    if ([text containsString:@"no document"] || [text containsString:@"no paper"] || [text containsString:@"empty"]) {
        return self.sides == 0 ? FMScanEmpty : FMScanDone;
    }
    return FMScanFailed;
}

- (NSString *)describe {
    NSMutableString *text = [NSMutableString string];
    [text appendFormat:@"scanners attached to this Mac: %lu\n", (unsigned long)self.found.count];
    for (ICScannerDevice *device in self.found) {
        [text appendFormat:@"- %@ (%@), session %@%@\n", device.name, device.transportType ?: @"?",
            device.hasOpenSession ? @"open" : @"closed",
            [self.discarded containsObject:device] ? @", left aside" : @""];
        if (!device.hasOpenSession) continue;
        [text appendFormat:@"  units: %@\n", [device.availableFunctionalUnitTypes componentsJoinedByString:@", "]];
        ICScannerFunctionalUnit *unit = device.selectedFunctionalUnit;
        [text appendFormat:@"  chosen unit: %lu (3 is the feeder)\n", (unsigned long)unit.type];
        NSMutableArray *resolutions = [NSMutableArray array];
        [unit.supportedResolutions enumerateIndexesUsingBlock:^(NSUInteger index, BOOL *stop) {
            [resolutions addObject:@(index)];
        }];
        [text appendFormat:@"  resolutions: %@\n", [resolutions componentsJoinedByString:@", "]];
        if (unit.type == ICScannerFunctionalUnitTypeDocumentFeeder) {
            ICScannerFunctionalUnitDocumentFeeder *feeder = (ICScannerFunctionalUnitDocumentFeeder *)unit;
            [text appendFormat:@"  both sides: %@\n", feeder.supportsDuplexScanning ? @"yes" : @"no"];
            [text appendFormat:@"  sheets in the feeder: %@\n", feeder.documentLoaded ? @"yes" : @"no"];
        }
    }
    static const char *names[] = {"off", "empty", "ready"};
    [text appendFormat:@"state: %s\n", names[[self state]]];
    [text appendString:@"events:\n"];
    for (NSString *event in self.events) [text appendFormat:@"  %@\n", event];
    return text;
}

#pragma mark - the scanners that come and go

- (void)deviceBrowser:(ICDeviceBrowser *)browser didAddDevice:(ICDevice *)device moreComing:(BOOL)moreComing {
    if (!(device.type & ICDeviceTypeScanner)) return;
    [self note:@"found %@", device.name];
    [self.found addObject:(ICScannerDevice *)device];
    [self openNext];
}

- (void)deviceBrowser:(ICDeviceBrowser *)browser didRemoveDevice:(ICDevice *)device moreGoing:(BOOL)moreGoing {
    [self note:@"gone %@", device.name];
    [self.found removeObject:(ICScannerDevice *)device];
    [self.discarded removeObject:(ICScannerDevice *)device];
    if (self.scanner != device) return;
    self.scanner = nil;
    self.feeder = NO;
    [self finish:FMScanOff message:nil];
    [self openNext];
}

- (void)didRemoveDevice:(ICDevice *)device {
}

#pragma mark - the session

- (void)device:(ICDevice *)device didOpenSessionWithError:(NSError *)error {
    if (error) [self discard:(ICScannerDevice *)device because:error.localizedDescription];
}

- (void)deviceDidBecomeReady:(ICDevice *)device {
    ICScannerDevice *scanner = (ICScannerDevice *)device;
    if (scanner != self.scanner) return;
    if (![scanner.availableFunctionalUnitTypes containsObject:@(ICScannerFunctionalUnitTypeDocumentFeeder)]) {
        [self discard:scanner because:@"no feeder"];
        return;
    }
    [self note:@"%@ ready, choosing the feeder", scanner.name];
    [scanner requestSelectFunctionalUnit:ICScannerFunctionalUnitTypeDocumentFeeder];
}

- (void)device:(ICDevice *)device didCloseSessionWithError:(NSError *)error {
    [self note:@"session closed: %@", error.localizedDescription ?: @"-"];
    if (self.scanner != device) return;
    self.scanner = nil;
    self.feeder = NO;
    [self finish:FMScanOff message:nil];
}

// Another program let the scanner go: it can be opened again.
- (void)scannerDeviceDidBecomeAvailable:(ICScannerDevice *)scanner {
    [self note:@"%@ available again", scanner.name];
    [self.discarded removeObject:scanner];
    [self openNext];
}

- (void)device:(ICDevice *)device didEncounterError:(NSError *)error {
    [self note:@"error: %@ (%ld)", error.localizedDescription, (long)error.code];
}

- (void)device:(ICDevice *)device didReceiveStatusInformation:(NSDictionary<ICDeviceStatus, id> *)status {
    [self note:@"status: %@", [[status description] stringByReplacingOccurrencesOfString:@"\n" withString:@" "]];
}

#pragma mark - the scan

// The answer arrives twice: first with no unit (the one before was let go), then with the feeder.
// A scan asked before the second is ignored without an error: the feeder counts as chosen only from there.
- (void)scannerDevice:(ICScannerDevice *)scanner didSelectFunctionalUnit:(ICScannerFunctionalUnit *)unit error:(NSError *)error {
    if (scanner != self.scanner) return;
    if (error) {
        [self discard:scanner because:error.localizedDescription];
        return;
    }
    if (unit.type != ICScannerFunctionalUnitTypeDocumentFeeder) return;
    self.feeder = YES;
    [self note:@"feeder chosen"];
}

- (void)scannerDevice:(ICScannerDevice *)scanner didScanToURL:(NSURL *)url {
    self.sides += 1;
    goScanPage((char *)url.path.UTF8String);
}

- (void)scannerDevice:(ICScannerDevice *)scanner didCompleteScanWithError:(NSError *)error {
    if (!error) {
        [self finish:FMScanDone message:nil];
        return;
    }
    NSString *message = [NSString stringWithFormat:@"%@ (%@ %ld)", error.localizedDescription, error.domain, (long)error.code];
    [self finish:[self codeOf:error] message:message];
}

- (void)scannerDevice:(ICScannerDevice *)scanner didCompleteOverviewScanWithError:(NSError *)error {
}

@end

static FMScanner *shared(void) {
    static FMScanner *scanner;
    if (!scanner) scanner = [[FMScanner alloc] init];
    return scanner;
}

void fmScannerStart(void) {
    onMain(^{ [shared() start]; });
}

void fmScannerStop(void) {
    onMain(^{ [shared() stop]; });
}

int fmScannerState(void) {
    __block int state = FMScannerOff;
    onMain(^{ state = [shared() state]; });
    return state;
}

int fmScannerScan(const char *dir, int resolution) {
    NSString *directory = [NSString stringWithUTF8String:dir];
    __block int code = FMScanFailed;
    onMain(^{ code = [shared() scanInto:directory resolution:(NSUInteger)resolution]; });
    return code;
}

void fmScannerCancel(void) {
    onMain(^{ [shared() cancel]; });
}

char *fmScannerDescribe(void) {
    __block char *text = NULL;
    onMain(^{ text = strdup([shared() describe].UTF8String); });
    return text;
}

void fmRunLoop(double seconds) {
    [[NSRunLoop mainRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:seconds]];
}
