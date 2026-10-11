// The windows of the agent on the Mac: a question, a message, and a message with buttons to choose from.
#import <AppKit/AppKit.h>
#include "agent_darwin.h"
#include "_cgo_export.h"

// The window of fmChoose while it is open, and its button that does not close it.
static NSAlert *choosing;
static NSButton *staying;

// A click on the button that leaves the window open turns every button off and tells Go, which
// answers with fmChooseDone when it has finished.
@interface FMStay : NSObject
- (void)clicked:(NSButton *)sender;
@end

@implementation FMStay
- (void)clicked:(NSButton *)sender {
    for (NSButton *button in choosing.buttons) button.enabled = NO;
    goChooseStay();
}
@end

// On the main thread, as a block of the run loop: the scanner keeps answering while the window is open.
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

// Shows the window and answers the place of the button clicked, from 0, or -1 when the window was
// closed without a click (fmCloseWindow). The button at the place stay, when there is one, does
// not close the window.
static int show(const char *title, const char *text, const char **buttons, int count, int stay) {
    NSString *message = [NSString stringWithUTF8String:title];
    NSString *detail = [NSString stringWithUTF8String:text];
    NSMutableArray<NSString *> *titles = [NSMutableArray array];
    for (int i = 0; i < count; i++) [titles addObject:[NSString stringWithUTF8String:buttons[i]]];
    __block NSModalResponse response = NSAlertFirstButtonReturn - 1;
    onMain(^{
        // the agent has no window and no icon in the Dock: without this the question stays behind the browser
        [NSApplication sharedApplication];
        [NSApp activateIgnoringOtherApps:YES];
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = message;
        alert.informativeText = detail;
        for (NSString *button in titles) [alert addButtonWithTitle:button];
        alert.window.level = NSFloatingWindowLevel;
        if (stay >= 0 && stay < (int)alert.buttons.count) {
            static FMStay *target;
            if (!target) target = [[FMStay alloc] init];
            choosing = alert;
            staying = alert.buttons[stay];
            staying.target = target;
            staying.action = @selector(clicked:);
        }
        response = [alert runModal];
        if (choosing == alert) {
            choosing = nil;
            staying = nil;
        }
    });
    return response < NSAlertFirstButtonReturn ? -1 : (int)(response - NSAlertFirstButtonReturn);
}

int fmAsk(const char *title, const char *text, const char *yes, const char *no) {
    const char *buttons[] = {yes, no};
    return show(title, text, buttons, 2, -1) == 0;
}

void fmTell(const char *title, const char *text) {
    const char *buttons[] = {"OK"};
    show(title, text, buttons, 1, -1);
}

int fmChoose(const char *title, const char *text, const char **buttons, int count, int stay) {
    return show(title, text, buttons, count, stay);
}

void fmChooseDone(const char *title) {
    NSString *name = [NSString stringWithUTF8String:title];
    onMain(^{
        if (!choosing) return;
        staying.title = name;
        for (NSButton *button in choosing.buttons) button.enabled = YES;
    });
}

int fmCloseWindow(void) {
    __block int closed = 0;
    void (^block)(void) = ^{
        closed = NSApp.modalWindow != nil;
    };
    if ([NSThread isMainThread]) {
        block();
    } else {
        onMain(block);
    }
    // after the answer: whoever waits for it is not left waiting, whatever closing does
    if (closed) {
        CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, ^{
            if (NSApp.modalWindow) [NSApp abortModal];
        });
        CFRunLoopWakeUp(CFRunLoopGetMain());
    }
    return closed;
}
