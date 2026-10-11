// The windows of the agent on the Mac: a question and a message.
#import <AppKit/AppKit.h>
#include "agent_darwin.h"

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

static NSModalResponse show(const char *title, const char *text, const char *first, const char *second) {
    NSString *message = [NSString stringWithUTF8String:title];
    NSString *detail = [NSString stringWithUTF8String:text];
    NSString *yes = [NSString stringWithUTF8String:first];
    NSString *no = second ? [NSString stringWithUTF8String:second] : nil;
    __block NSModalResponse response = NSAlertSecondButtonReturn;
    onMain(^{
        // the agent has no window and no icon in the Dock: without this the question stays behind the browser
        [NSApplication sharedApplication];
        [NSApp activateIgnoringOtherApps:YES];
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = message;
        alert.informativeText = detail;
        [alert addButtonWithTitle:yes];
        if (no) [alert addButtonWithTitle:no];
        alert.window.level = NSFloatingWindowLevel;
        response = [alert runModal];
    });
    return response;
}

int fmAsk(const char *title, const char *text, const char *yes, const char *no) {
    return show(title, text, yes, no) == NSAlertFirstButtonReturn;
}

void fmTell(const char *title, const char *text) {
    show(title, text, "OK", NULL);
}
