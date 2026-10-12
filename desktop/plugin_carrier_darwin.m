#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import "_cgo_export.h"

@interface PluginWebView : WKWebView
@property(nonatomic, weak) NSResponder *workbenchResponder;
@end
@implementation PluginWebView
@end

@interface PluginSurface : NSView
@end
@implementation PluginSurface
- (BOOL)isFlipped { return YES; }
- (NSView *)hitTest:(NSPoint)point {
    if (self.hidden || !NSPointInRect([self convertPoint:point fromView:self.superview], self.bounds)) return nil;
    return [super hitTest:point];
}
@end

@interface PluginPage : NSObject <WKScriptMessageHandler, WKNavigationDelegate, WKUIDelegate>
@property(nonatomic, strong) PluginWebView *view;
@property(nonatomic, strong) PluginSurface *surface;
@property(nonatomic) uintptr_t handle;
@property(nonatomic) BOOL active;
@property(nonatomic) NSUInteger messageLimit;
@end

@implementation PluginPage
- (void)publish:(NSString *)body {
    if (!self.active) return;
    receivePluginPage(self.handle, (char *)body.UTF8String);
}

- (void)userContentController:(WKUserContentController *)controller
     didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!self.active || !message.frameInfo.isMainFrame) return;
    if (@available(macOS 13.0, *)) {
        if (!self.view.configuration.defaultWebpagePreferences.isLockdownModeEnabled) {
            [self publish:@"{\"type\":\"failure\",\"reason\":\"Native isolation configuration was lost.\"}"];
            return;
        }
    }
    if (![message.body isKindOfClass:[NSString class]] || [(NSString *)message.body lengthOfBytesUsingEncoding:NSUTF8StringEncoding] > self.messageLimit) {
        [self publish:@"{\"type\":\"failure\",\"reason\":\"Invalid native carrier message.\"}"];
        return;
    }
    [self publish:message.body];
}

- (void)webView:(WKWebView *)view didFailProvisionalNavigation:(WKNavigation *)navigation
     withError:(NSError *)error {
    [self publish:@"{\"type\":\"failure\",\"reason\":\"Native plugin page could not load.\"}"];
}

- (void)webView:(WKWebView *)view didFailNavigation:(WKNavigation *)navigation
     withError:(NSError *)error {
    [self publish:@"{\"type\":\"failure\",\"reason\":\"Native plugin page navigation failed.\"}"];
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)view {
    [self publish:@"{\"type\":\"failure\",\"reason\":\"Native plugin process terminated.\"}"];
}

- (WKWebView *)webView:(WKWebView *)view
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
    forNavigationAction:(WKNavigationAction *)action
    windowFeatures:(WKWindowFeatures *)features {
    return nil;
}
@end

void *createPluginPage(void *windowHandle, uintptr_t handle, size_t messageLimit) {
    if (!NSThread.isMainThread || windowHandle == NULL) return NULL;
    if (@available(macOS 13.0, *)) {
        NSWindow *window = (__bridge NSWindow *)windowHandle;
        PluginPage *page = [PluginPage new];
        page.handle = handle;
        page.active = YES;
        page.messageLimit = messageLimit;
        NSResponder *workbenchResponder = window.firstResponder;
        if ([workbenchResponder isKindOfClass:[NSView class]]) {
            NSView *focused = (NSView *)workbenchResponder;
            while (focused) {
                if ([focused isKindOfClass:[PluginWebView class]]) {
                    workbenchResponder = [(PluginWebView *)focused workbenchResponder];
                    break;
                }
                focused = focused.superview;
            }
        }

        WKWebViewConfiguration *config = [WKWebViewConfiguration new];
        config.defaultWebpagePreferences.lockdownModeEnabled = YES;
        config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
        [config.userContentController addScriptMessageHandler:page name:@"carrier"];
        page.view = [[PluginWebView alloc] initWithFrame:NSZeroRect configuration:config];
        page.view.workbenchResponder = workbenchResponder;
        page.view.navigationDelegate = page;
        page.view.UIDelegate = page;
        page.surface = [PluginSurface new];
        page.surface.wantsLayer = YES;
        page.surface.layer.masksToBounds = YES;
        [page.surface addSubview:page.view];
        [window.contentView addSubview:page.surface];
        return (__bridge_retained void *)page;
    }
    return NULL;
}

void loadPluginPage(void *pointer, const char *html) {
    PluginPage *page = (__bridge PluginPage *)pointer;
    [page.view loadHTMLString:[NSString stringWithUTF8String:html] baseURL:nil];
}

static void restorePluginFocus(PluginPage *page) {
    NSWindow *window = page.view.window;
    NSResponder *focused = window.firstResponder;
    if ([focused isKindOfClass:[NSView class]] && [(NSView *)focused isDescendantOf:page.view]) {
        NSResponder *previous = page.view.workbenchResponder;
        if (previous == window || ([previous isKindOfClass:[NSView class]] &&
            [(NSView *)previous window] == window && ![(NSView *)previous isHiddenOrHasHiddenAncestor])) {
            [window makeFirstResponder:previous];
        } else {
            [window makeFirstResponder:nil];
        }
    }
}

void positionPluginPage(void *pointer, double x, double y, double width, double height,
                        double clipX, double clipY, double clipWidth, double clipHeight) {
    PluginPage *page = (__bridge PluginPage *)pointer;
    NSView *parent = page.surface.superview;
    NSRect requested = NSMakeRect(clipX, parent.bounds.size.height - clipY - clipHeight, clipWidth, clipHeight);
    NSRect visible = NSIntersectionRect(requested, parent.bounds);
    BOOL hidden = NSIsEmptyRect(visible);
    if (hidden) restorePluginFocus(page);
    page.surface.hidden = hidden;
    page.surface.frame = visible;
    double top = parent.bounds.size.height - NSMaxY(visible);
    page.view.frame = NSMakeRect(x - visible.origin.x, y - top, width, height);
}

void sendPluginPage(void *pointer, const char *body) {
    PluginPage *page = (__bridge PluginPage *)pointer;
    NSString *script = [NSString stringWithFormat:@"window.flameCarrierReceive(%@)",
                        [NSString stringWithUTF8String:body]];
    [page.view evaluateJavaScript:script completionHandler:^(id result, NSError *error) {
        if (error) [page publish:@"{\"type\":\"failure\",\"reason\":\"Native plugin publication failed.\"}"];
    }];
}

void closePluginPage(void *pointer) {
    PluginPage *page = (__bridge_transfer PluginPage *)pointer;
    restorePluginFocus(page);
    page.active = NO;
    [page.view.configuration.userContentController removeScriptMessageHandlerForName:@"carrier"];
    [page.view.configuration.userContentController removeAllUserScripts];
    page.view.navigationDelegate = nil;
    page.view.UIDelegate = nil;
    [page.view stopLoading];
    [page.surface removeFromSuperview];
    page.view = nil;
    page.surface = nil;
}
