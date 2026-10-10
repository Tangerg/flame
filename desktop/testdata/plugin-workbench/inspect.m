//go:build nativeworkbenchprobe && darwin

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <Security/Security.h>
void receiveWorkbenchEvaluation(uintptr_t, char *);

char *workbenchSandboxHome(const char *identity, const char *fixture, const char *canary) {
    NSString *expected = [NSString stringWithUTF8String:identity];
    SecTaskRef task = SecTaskCreateFromSelf(NULL);
    CFTypeRef sandbox = SecTaskCopyValueForEntitlement(task, CFSTR("com.apple.security.app-sandbox"), NULL);
    BOOL enabled = sandbox && CFEqual(sandbox, kCFBooleanTrue);
    if (sandbox) CFRelease(sandbox);
    CFRelease(task);
    NSString *home = NSHomeDirectory();
    NSString *library = NSSearchPathForDirectoriesInDomains(NSLibraryDirectory, NSUserDomainMask, YES).firstObject;
    NSString *suffix = [NSString stringWithFormat:@"/Library/Containers/%@/Data", expected];
    NSData *allowed = [NSData dataWithContentsOfFile:[NSString stringWithUTF8String:fixture]];
    NSData *denied = [NSData dataWithContentsOfFile:[NSString stringWithUTF8String:canary]];
    if (!enabled || ![NSBundle.mainBundle.bundleIdentifier isEqualToString:expected] ||
        ![home hasSuffix:suffix] || ![library isEqualToString:[home stringByAppendingPathComponent:@"Library"]] ||
        !allowed || denied) {
        fprintf(stderr, "native workbench isolation: sandbox=%d identity=%s home=%s library=%s fixture=%d canaryDenied=%d\n",
            enabled, NSBundle.mainBundle.bundleIdentifier.UTF8String, home.UTF8String, library.UTF8String, allowed != nil, denied == nil);
        return NULL;
    }
    return strdup(home.UTF8String);
}

@interface WorkbenchPageObserver : NSObject <WKScriptMessageHandler>
@property(nonatomic, strong) WKFrameInfo *guest;
@end
@implementation WorkbenchPageObserver
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!message.frameInfo.isMainFrame) self.guest = message.frameInfo;
}
@end

static NSMapTable *observers;

// Test-only inspection enters WebKit's frame API; it grants no Runtime authority.
void observeWorkbenchPage(void *pointer) {
    WKWebViewConfiguration *config = [(WKWebView *)[(__bridge id)pointer valueForKey:@"view"] configuration];
    if (!observers) observers = [NSMapTable weakToStrongObjectsMapTable];
    WorkbenchPageObserver *observer = [WorkbenchPageObserver new];
    [observers setObject:observer forKey:config.userContentController];
    [config.userContentController addScriptMessageHandler:observer name:@"nativeWorkbenchGate"];
    WKUserScript *script = [[WKUserScript alloc] initWithSource:@"if (window !== top) window.webkit.messageHandlers.nativeWorkbenchGate.postMessage('ready')"
        injectionTime:WKUserScriptInjectionTimeAtDocumentEnd forMainFrameOnly:NO];
    [config.userContentController addUserScript:script];
}

void stopObservingWorkbenchPage(void *pointer) {
    WKWebViewConfiguration *config = [(WKWebView *)[(__bridge id)pointer valueForKey:@"view"] configuration];
    [config.userContentController removeScriptMessageHandlerForName:@"nativeWorkbenchGate"];
    [observers removeObjectForKey:config.userContentController];
}

static WKWebView *workbenchView(void *pointer) {
    return [(__bridge id)pointer valueForKey:@"view"];
}

bool workbenchPageReady(void *pointer) {
    WKWebView *view = workbenchView(pointer);
    return [(WorkbenchPageObserver *)[observers objectForKey:view.configuration.userContentController] guest] != nil;
}

bool workbenchPageVisible(void *pointer) {
    WKWebView *view = workbenchView(pointer);
    return !view.isHiddenOrHasHiddenAncestor && !NSIsEmptyRect(view.frame);
}

static int countPages(NSView *view) {
    int count = [view isKindOfClass:NSClassFromString(@"PluginWebView")] ? 1 : 0;
    for (NSView *child in view.subviews) count += countPages(child);
    return count;
}

int workbenchPhysicalPages(void *window) {
    return countPages([(__bridge NSWindow *)window contentView]);
}

void evaluateWorkbenchPage(void *pointer, const char *source, uintptr_t handle) {
    WKWebView *view = workbenchView(pointer);
    WorkbenchPageObserver *observer = [observers objectForKey:view.configuration.userContentController];
    [view evaluateJavaScript:[NSString stringWithUTF8String:source] inFrame:observer.guest inContentWorld:WKContentWorld.pageWorld
        completionHandler:^(id result, NSError *error) {
            NSString *body = error ? [NSString stringWithFormat:@"{\"error\":%@}",
                [[NSString alloc] initWithData:[NSJSONSerialization dataWithJSONObject:error.localizedDescription options:NSJSONWritingFragmentsAllowed error:nil] encoding:NSUTF8StringEncoding]] :
                [[NSString alloc] initWithData:[NSJSONSerialization dataWithJSONObject:result ?: NSNull.null options:NSJSONWritingFragmentsAllowed error:nil] encoding:NSUTF8StringEncoding];
            receiveWorkbenchEvaluation(handle, (char *)body.UTF8String);
        }];
}
