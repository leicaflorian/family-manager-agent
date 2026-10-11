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
// the scanners seen, in the order of arrival: the ones of the network too
@property(strong) NSMutableArray<ICScannerDevice *> *found;
// for the page of before, the ones that cannot be used: no feeder, or the session did not open
@property(strong) NSMutableArray<ICScannerDevice *> *discarded;
// the scanner asked: its id, "" for the page of before (the first on the cable with a feeder),
// nil until a page asks. Nothing is opened before
@property(copy) NSString *wanted;
@property int wantedUnit;
// the one whose session is open, or opening
@property(strong) ICScannerDevice *scanner;
// the ones let go whose session is still closing: one of them asked again is opened when it has closed
@property(strong) NSMutableArray<ICScannerDevice *> *closing;
// its session is open: a unit can be chosen
@property BOOL opened;
// the unit asked of it, and whether the scanner confirmed it: only then a scan is accepted
@property ICScannerFunctionalUnitType unitType;
@property BOOL unitReady;
// the scanner asked did not open
@property BOOL failed;
@property(strong) NSDate *tried;
@property BOOL scanning;
@property NSUInteger sides;
@property(strong) NSMutableArray<NSString *> *events;
@end

// After how long a scanner asked by its id that did not open, or closed, is tried again.
static const NSTimeInterval retryAfter = 5;

@implementation FMScanner

- (instancetype)init {
    self = [super init];
    _found = [NSMutableArray array];
    _discarded = [NSMutableArray array];
    _closing = [NSMutableArray array];
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
    // the scanners of the network too: they are listed, and opened only when the page asks for
    // one, because opening a session wakes them up
    self.browser.browsedDeviceTypeMask = ICDeviceTypeMaskScanner | ICDeviceLocationTypeMaskLocal |
        ICDeviceLocationTypeMaskShared | ICDeviceLocationTypeMaskBonjour;
    [self.browser start];
}

- (void)stop {
    if (self.scanning) [self.scanner cancelScan];
    [self letGo];
    [self.browser stop];
    self.browser = nil;
    self.wanted = nil;
}

// A scanner reached through the network, or shared by another Mac: not attached to this one.
- (BOOL)onNetwork:(ICDevice *)device {
    return ![device.transportType isEqualToString:ICTransportTypeUSB];
}

// The name the page keeps of a scanner: the same at the next start of the Mac.
- (NSString *)idOf:(ICDevice *)device {
    return device.persistentIDString ?: device.UUIDString ?: device.name ?: @"";
}

- (NSString *)list {
    NSMutableString *text = [NSMutableString string];
    for (ICScannerDevice *device in self.found) {
        NSString *name = [device.name stringByReplacingOccurrencesOfString:@"\t" withString:@" "];
        [text appendFormat:@"%@\t%@\t%@\n", [self idOf:device], [self onNetwork:device] ? @"network" : @"usb", name ?: @""];
    }
    return text;
}

// Leaves the scanner to the other programs.
- (void)letGo {
    ICScannerDevice *before = self.scanner;
    self.scanner = nil;
    self.opened = NO;
    self.unitReady = NO;
    if (!before.hasOpenSession) return;
    if (![self.closing containsObject:before]) [self.closing addObject:before];
    [before requestCloseSession];
}

// The page asked for a scanner, and where it takes the sheets from. Another scanner than the one
// before: that one is let go. The same one, set on another unit: only the unit changes.
- (void)choose:(NSString *)identifier unit:(int)unit {
    if (self.scanning) return;
    if ([identifier isEqualToString:self.wanted]) {
        if (unit != self.wantedUnit) {
            self.wantedUnit = unit;
            if (self.opened) [self selectUnit];
        }
        // the one asked by its id did not open, or closed: it may be back
        if (!self.scanner && identifier.length && -self.tried.timeIntervalSinceNow > retryAfter) {
            self.tried = [NSDate date];
            [self openNext];
        }
        return;
    }
    [self letGo];
    self.wanted = identifier;
    self.wantedUnit = unit;
    self.failed = NO;
    self.tried = [NSDate date];
    [self.discarded removeAllObjects];
    [self note:@"asked for %@", identifier.length ? identifier : @"the first on the cable with a feeder"];
    [self openNext];
}

// Opens the scanner asked. For the page of before, the first on the cable not yet discarded, one
// at a time: whether it has a feeder is known only with the session open.
- (void)openNext {
    if (self.scanner || !self.wanted) return;
    for (ICScannerDevice *candidate in self.found) {
        if (self.wanted.length) {
            if (![[self idOf:candidate] isEqualToString:self.wanted]) continue;
        } else if ([self.discarded containsObject:candidate] || [self onNetwork:candidate]) {
            continue;
        }
        // let go a moment ago, by a page that asks for it again (or by the page of before after the
        // one that chooses): a session asked while the one before closes is closed with it
        if ([self.closing containsObject:candidate]) return;
        self.scanner = candidate;
        self.opened = NO;
        self.unitReady = NO;
        self.failed = NO;
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
        [self letGo];
        self.failed = YES;
        self.tried = [NSDate date];
    }
    // the page of before goes on to the next scanner; the one asked by its id is the only one
    if (!self.wanted.length) [self openNext];
}

// Sets the open scanner on the unit asked: the feeder, or the glass when that was asked or there
// is no feeder. The page of before knows only the feeder.
- (void)selectUnit {
    ICScannerDevice *scanner = self.scanner;
    NSArray<NSNumber *> *types = scanner.availableFunctionalUnitTypes;
    BOOL feeder = [types containsObject:@(ICScannerFunctionalUnitTypeDocumentFeeder)];
    BOOL flatbed = self.wanted.length && [types containsObject:@(ICScannerFunctionalUnitTypeFlatbed)];
    if (!feeder && !flatbed) {
        [self discard:scanner because:self.wanted.length ? @"neither a feeder nor a glass" : @"no feeder"];
        return;
    }
    BOOL glass = flatbed && (self.wantedUnit == FMUnitFlatbed || !feeder);
    ICScannerFunctionalUnitType type = glass ? ICScannerFunctionalUnitTypeFlatbed : ICScannerFunctionalUnitTypeDocumentFeeder;
    // already set on it: a glass asked of a scanner that has only the feeder changes nothing
    if (self.unitReady && type == self.unitType) return;
    self.unitType = type;
    self.unitReady = NO;
    [self note:@"%@ ready, choosing the %@", scanner.name, glass ? @"glass" : @"feeder"];
    [scanner requestSelectFunctionalUnit:self.unitType];
}

// The unit the scanner is set on, once it confirmed it.
- (ICScannerFunctionalUnit *)unit {
    if (!self.unitReady) return nil;
    ICScannerFunctionalUnit *unit = self.scanner.selectedFunctionalUnit;
    return unit.type == self.unitType ? unit : nil;
}

- (ICScannerFunctionalUnitDocumentFeeder *)feederUnit {
    ICScannerFunctionalUnit *unit = [self unit];
    if (unit.type != ICScannerFunctionalUnitTypeDocumentFeeder) return nil;
    return (ICScannerFunctionalUnitDocumentFeeder *)unit;
}

- (int)state {
    ICScannerFunctionalUnit *unit = [self unit];
    if (!unit) {
        if (self.failed) return FMScannerOff;
        // open, opening, or waiting for its session of before to close
        return self.scanner || (self.wanted && self.closing.count) ? FMScannerOpening : FMScannerOff;
    }
    // the glass does not say whether a sheet is on it
    if (unit.type != ICScannerFunctionalUnitTypeDocumentFeeder) return FMScannerReady;
    return ((ICScannerFunctionalUnitDocumentFeeder *)unit).documentLoaded ? FMScannerReady : FMScannerEmpty;
}

- (NSString *)has {
    ICScannerFunctionalUnit *unit = [self unit];
    if (!unit) return @"";
    NSMutableArray<NSString *> *units = [NSMutableArray array];
    NSArray<NSNumber *> *types = self.scanner.availableFunctionalUnitTypes;
    if ([types containsObject:@(ICScannerFunctionalUnitTypeDocumentFeeder)]) [units addObject:@"feeder"];
    if ([types containsObject:@(ICScannerFunctionalUnitTypeFlatbed)]) [units addObject:@"flatbed"];
    NSMutableArray<NSString *> *resolutions = [NSMutableArray array];
    [unit.supportedResolutions enumerateIndexesUsingBlock:^(NSUInteger index, BOOL *stop) {
        [resolutions addObject:[NSString stringWithFormat:@"%lu", (unsigned long)index]];
    }];
    return [NSString stringWithFormat:@"%@\t%@\t%@",
        unit.type == ICScannerFunctionalUnitTypeDocumentFeeder ? @"feeder" : @"flatbed",
        [units componentsJoinedByString:@","], [resolutions componentsJoinedByString:@","]];
}

- (void)cancel {
    if (!self.scanning) return;
    self.scanning = NO;
    [self note:@"scan cancelled"];
    [self.scanner cancelScan];
}

- (int)scanInto:(NSString *)directory resolution:(NSUInteger)resolution grey:(BOOL)grey {
    ICScannerFunctionalUnit *unit = [self unit];
    if (!unit) return FMScanOff;
    if (self.scanning) return FMScanFailed;
    // a resolution the scanner does not have stops the scan: it is not replaced with another
    if (unit.supportedResolutions.count && ![unit.supportedResolutions containsIndex:resolution]) return FMScanSettings;

    ICScannerFunctionalUnitDocumentFeeder *feeder = [self feederUnit];
    if (feeder) {
        feeder.documentType = ICScannerDocumentTypeA4;
        if (feeder.supportsDuplexScanning) feeder.duplexScanningEnabled = YES;
    } else {
        // from the glass, the area of an A4 from its corner, or the whole glass when it is smaller
        unit.measurementUnit = ICScannerMeasurementUnitInches;
        NSSize glass = unit.physicalSize;
        CGFloat width = 210 / 25.4, height = 297 / 25.4;
        if (glass.width > 0 && glass.width < width) width = glass.width;
        if (glass.height > 0 && glass.height < height) height = glass.height;
        unit.scanArea = NSMakeRect(0, 0, width, height);
    }
    unit.resolution = resolution;
    unit.pixelDataType = grey ? ICScannerPixelDataTypeGray : ICScannerPixelDataTypeRGB;
    unit.bitDepth = ICScannerBitDepth8Bits;

    self.scanner.transferMode = ICScannerTransferModeFileBased;
    self.scanner.downloadsDirectory = [NSURL fileURLWithPath:directory isDirectory:YES];
    self.scanner.documentName = @"side";
    self.scanner.documentUTI = @"public.jpeg";

    self.scanning = YES;
    self.sides = 0;
    [self note:@"scan asked: %@, %lu dots, %@, both sides %d, sheets %d", feeder ? @"feeder" : @"glass",
        (unsigned long)unit.resolution, grey ? @"grey" : @"colour", feeder.duplexScanningEnabled, feeder.documentLoaded];
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
    [text appendFormat:@"scanners this Mac sees: %lu\n", (unsigned long)self.found.count];
    for (ICScannerDevice *device in self.found) {
        [text appendFormat:@"- %@ (%@, %@), id %@, session %@%@\n", device.name, device.transportType ?: @"?",
            [self onNetwork:device] ? @"network" : @"usb", [self idOf:device], device.hasOpenSession ? @"open" : @"closed",
            [self.discarded containsObject:device] ? @", left aside" : @""];
        if (!device.hasOpenSession) continue;
        [text appendFormat:@"  units: %@ (0 is the glass, 3 the feeder)\n", [device.availableFunctionalUnitTypes componentsJoinedByString:@", "]];
        ICScannerFunctionalUnit *unit = device.selectedFunctionalUnit;
        [text appendFormat:@"  chosen unit: %lu\n", (unsigned long)unit.type];
        NSMutableArray *resolutions = [NSMutableArray array];
        [unit.supportedResolutions enumerateIndexesUsingBlock:^(NSUInteger index, BOOL *stop) {
            [resolutions addObject:@(index)];
        }];
        [text appendFormat:@"  resolutions: %@\n", [resolutions componentsJoinedByString:@", "]];
        if (unit.type == ICScannerFunctionalUnitTypeDocumentFeeder) {
            ICScannerFunctionalUnitDocumentFeeder *feeder = (ICScannerFunctionalUnitDocumentFeeder *)unit;
            [text appendFormat:@"  both sides: %@\n", feeder.supportsDuplexScanning ? @"yes" : @"no"];
            [text appendFormat:@"  sheets in the feeder: %@\n", feeder.documentLoaded ? @"yes" : @"no"];
        } else {
            [text appendFormat:@"  glass: %.2f x %.2f (unit of measure %lu, 0 is inches)\n", unit.physicalSize.width,
                unit.physicalSize.height, (unsigned long)unit.measurementUnit];
        }
    }
    static const char *names[] = {"off", "empty", "ready", "opening"};
    [text appendFormat:@"asked: %@\n", !self.wanted ? @"nothing" : self.wanted.length ? self.wanted : @"the first on the cable with a feeder"];
    [text appendFormat:@"state: %s\n", names[[self state]]];
    [text appendString:@"events:\n"];
    for (NSString *event in self.events) [text appendFormat:@"  %@\n", event];
    return text;
}

#pragma mark - the scanners that come and go

- (void)deviceBrowser:(ICDeviceBrowser *)browser didAddDevice:(ICDevice *)device moreComing:(BOOL)moreComing {
    if (!(device.type & ICDeviceTypeScanner)) return;
    [self note:@"found %@ (%@)", device.name, device.transportType ?: @"?"];
    [self.found addObject:(ICScannerDevice *)device];
    [self openNext];
}

- (void)deviceBrowser:(ICDeviceBrowser *)browser didRemoveDevice:(ICDevice *)device moreGoing:(BOOL)moreGoing {
    [self note:@"gone %@", device.name];
    [self.found removeObject:(ICScannerDevice *)device];
    [self.discarded removeObject:(ICScannerDevice *)device];
    [self.closing removeObject:(ICScannerDevice *)device];
    if (self.scanner != device) return;
    self.scanner = nil;
    self.opened = NO;
    self.unitReady = NO;
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
    if (scanner != self.scanner) {
        // the page asked for another one while this was opening: it is let go
        if (scanner.hasOpenSession) [scanner requestCloseSession];
        return;
    }
    self.opened = YES;
    [self selectUnit];
}

- (void)device:(ICDevice *)device didCloseSessionWithError:(NSError *)error {
    [self note:@"session closed: %@", error.localizedDescription ?: @"-"];
    if ([self.closing containsObject:(ICScannerDevice *)device]) {
        // let go by the agent: it may be the one asked now
        [self.closing removeObject:(ICScannerDevice *)device];
        [self openNext];
        return;
    }
    if (self.scanner != device) return;
    self.scanner = nil;
    self.opened = NO;
    self.unitReady = NO;
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

// The answer arrives twice: first with no unit (the one before was let go), then with the one asked.
// A scan asked before the second is ignored without an error: the unit counts as chosen only from
// there. The glass is unit 0, as no unit at all: the unit must be there.
- (void)scannerDevice:(ICScannerDevice *)scanner didSelectFunctionalUnit:(ICScannerFunctionalUnit *)unit error:(NSError *)error {
    if (scanner != self.scanner) return;
    if (error) {
        [self discard:scanner because:error.localizedDescription];
        return;
    }
    if (!unit || unit.type != self.unitType) return;
    self.unitReady = YES;
    [self note:@"%@ chosen", unit.type == ICScannerFunctionalUnitTypeDocumentFeeder ? @"feeder" : @"glass"];
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

char *fmScannerList(void) {
    __block char *text = NULL;
    onMain(^{ text = strdup([shared() list].UTF8String); });
    return text;
}

void fmScannerChoose(const char *id, int unit) {
    NSString *identifier = [NSString stringWithUTF8String:id] ?: @"";
    onMain(^{ [shared() choose:identifier unit:unit]; });
}

char *fmScannerHas(void) {
    __block char *text = NULL;
    onMain(^{ text = strdup([shared() has].UTF8String); });
    return text;
}

int fmScannerScan(const char *dir, int resolution, int grey) {
    NSString *directory = [NSString stringWithUTF8String:dir];
    __block int code = FMScanFailed;
    onMain(^{ code = [shared() scanInto:directory resolution:(NSUInteger)resolution grey:grey != 0]; });
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
