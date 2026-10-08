#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import "_cgo_export.h"

@interface IsolatedCarrier : NSObject <WKScriptMessageHandler, WKNavigationDelegate>
@property(nonatomic, strong) WKWebView *webView;
@property(nonatomic) uintptr_t handle;
@property(nonatomic) uint64_t rejected;
@property(nonatomic) BOOL active;
@end

@implementation IsolatedCarrier
- (void)complete:(NSString *)payload failed:(BOOL)failed {
    if (!self.active) return;
    // Go may close the carrier synchronously before this callback returns.
    __attribute__((objc_precise_lifetime)) IsolatedCarrier *lifetime = self;
    lifetime.active = NO;
    completeIsolatedProbe(lifetime.handle, (char *)payload.UTF8String, failed, lifetime.webView.configuration.defaultWebpagePreferences.isLockdownModeEnabled, lifetime.rejected);
}
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!self.active) return;
    if (!message.frameInfo.isMainFrame) {
        self.rejected++;
        return;
    }
    if (![message.body isKindOfClass:[NSString class]]) {
        [self fail:@"invalid native carrier result"];
        return;
    }
    [self complete:message.body failed:NO];
}
- (void)fail:(NSString *)reason {
    if (!self.active) return;
    [self complete:reason failed:YES];
}
- (void)webView:(WKWebView *)view didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self fail:error.localizedDescription];
}
- (void)webView:(WKWebView *)view didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self fail:error.localizedDescription];
}
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)view {
    [self fail:@"native carrier process terminated"];
}
@end

void *createIsolatedCarrier(void *windowHandle, const char *url, bool lockdown, uintptr_t handle) {
    if (!NSThread.isMainThread) return NULL;
    if (@available(macOS 13.0, *)) {
        NSWindow *window = (__bridge NSWindow *)windowHandle;
        IsolatedCarrier *carrier = [IsolatedCarrier new];
        carrier.handle = handle;
        carrier.active = YES;
        WKWebViewConfiguration *configuration = [WKWebViewConfiguration new];
        configuration.defaultWebpagePreferences.lockdownModeEnabled = lockdown;
        configuration.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
        [configuration.userContentController addScriptMessageHandler:carrier name:@"carrier"];
        NSString *publication = @"const output = document.getElementById('result'); new MutationObserver(() => { if (window.carrierResult !== undefined) window.webkit.messageHandlers.carrier.postMessage(output.textContent); }).observe(output, { childList: true, subtree: true, characterData: true });";
        [configuration.userContentController addUserScript:[[WKUserScript alloc] initWithSource:publication injectionTime:WKUserScriptInjectionTimeAtDocumentEnd forMainFrameOnly:YES]];
        carrier.webView = [[WKWebView alloc] initWithFrame:window.contentView.bounds configuration:configuration];
        carrier.webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
        carrier.webView.navigationDelegate = carrier;
        [window.contentView addSubview:carrier.webView];
        [carrier.webView loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:[NSString stringWithUTF8String:url]]]];
        return (__bridge_retained void *)carrier;
    }
    return NULL;
}

void closeIsolatedCarrier(void *pointer) {
    IsolatedCarrier *carrier = (__bridge_transfer IsolatedCarrier *)pointer;
    carrier.active = NO;
    [carrier.webView.configuration.userContentController removeScriptMessageHandlerForName:@"carrier"];
    [carrier.webView.configuration.userContentController removeAllUserScripts];
    carrier.webView.navigationDelegate = nil;
    [carrier.webView stopLoading];
    [carrier.webView removeFromSuperview];
    carrier.webView = nil;
}
