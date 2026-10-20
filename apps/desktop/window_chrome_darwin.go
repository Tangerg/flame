package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static WKWebView *findWebView(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) return (WKWebView *)view;
	for (NSView *subview in view.subviews) {
		WKWebView *found = findWebView(subview);
		if (found != nil) return found;
	}
	return nil;
}

static WKWebView *windowWebView(void *windowHandle) {
	NSWindow *window = (__bridge NSWindow *)windowHandle;
	if (window == nil || window.contentView == nil) return nil;
	return findWebView(window.contentView);
}

typedef struct {
	double controlsCentreY;
	double controlsInlineEnd;
	int measured;
} ChromeMetrics;

static ChromeMetrics measureOnMain(void *windowHandle) {
	ChromeMetrics metrics = {0, 0, 0};
	NSWindow *window = (__bridge NSWindow *)windowHandle;
	if (window == nil) return metrics;
	NSButton *close = [window standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
	WKWebView *webView = windowWebView(windowHandle);
	if (close == nil || zoom == nil || webView == nil) return metrics;
	// AppKit reports points; the page lays out in CSS pixels, which pageZoom scales.
	double scale = webView.pageZoom;

	// The buttons can keep isHidden false inside the fullscreen titlebar.
	// Fullscreen takes the marks away with the menu bar, and a gutter reserved for
	// something that is not there is a hole in the header. Zero says "no gutter",
	// and leaves the center at zero with it so nothing is aligned to a ghost.
	if (!(window.styleMask & NSWindowStyleMaskFullScreen) && !close.isHidden && !zoom.isHidden) {
		NSRect closeFrame = [close.superview convertRect:close.frame toView:nil];
		NSRect zoomFrame = [zoom.superview convertRect:zoom.frame toView:nil];
		metrics.controlsInlineEnd = NSMaxX(zoomFrame) / scale;
		// Window coordinates are bottom-left; CSS wants the distance down from the top.
		metrics.controlsCentreY = (window.frame.size.height - NSMidY(closeFrame)) / scale;
	}
	metrics.measured = 1;
	return metrics;
}

// AppKit may only be touched from the main thread. Wails runs a binding call on
// its own goroutine, so the work hops across; dispatch_sync from the main thread
// onto its own queue would deadlock, hence the check.
static ChromeMetrics windowChromeMetrics(void *windowHandle) {
	if ([NSThread isMainThread]) return measureOnMain(windowHandle);
	__block ChromeMetrics metrics;
	dispatch_sync(dispatch_get_main_queue(), ^{ metrics = measureOnMain(windowHandle); });
	return metrics;
}

// Wails' zoom roles set WKWebView magnification, which scales the viewport
// without reflowing the page. pageZoom reflows, and it is the only stored factor.
static void changePageZoomOnMain(void *windowHandle, int change) {
	WKWebView *webView = windowWebView(windowHandle);
	if (webView == nil) return;
	if (change > 0) {
		webView.pageZoom = webView.pageZoom + 0.05;
	} else if (change < 0 && webView.pageZoom > 1.05) {
		webView.pageZoom = webView.pageZoom - 0.05;
	} else {
		webView.pageZoom = 1.0;
	}
}

static void changePageZoom(void *windowHandle, int change) {
	if ([NSThread isMainThread]) {
		changePageZoomOnMain(windowHandle, change);
		return;
	}
	dispatch_sync(dispatch_get_main_queue(), ^{ changePageZoomOnMain(windowHandle, change); });
}

*/
import "C"

import "unsafe"

// nativeWindowChrome reports where the platform put the window's own controls, so
// the header the app draws around them can be laid out against the real numbers
// rather than against remembered ones.
//
// Web content cannot see AppKit's control geometry, and hiding the native buttons
// would discard platform behavior such as the zoom button's tiling menu. Two
// numbers cross the boundary, and they are the two the app cannot otherwise know:
// where the cluster ends, which is where the header's first control may begin, and
// the marks' center line, which is what that control centers on.
//
// Deliberately NOT the titlebar height, which would be the app's header height by
// another name — that number belongs to the visual style, and handing the frame a
// second claim on it would leave one value with two owners.
//
// The exact Wails window handle is passed in; inspecting an application-global
// window list could select a sheet or file dialog. A nil handle means no window yet,
// which is valid before creation and after destruction.
func nativeWindowChrome(window unsafe.Pointer) (controlsCentreY, controlsInlineEnd float64, measured bool) {
	metrics := C.windowChromeMetrics(window)
	return float64(metrics.controlsCentreY), float64(metrics.controlsInlineEnd), metrics.measured != 0
}

type pageZoomChange int

const (
	pageZoomReset pageZoomChange = 0
	pageZoomIn    pageZoomChange = 1
	pageZoomOut   pageZoomChange = -1
)

func changeNativePageZoom(window unsafe.Pointer, change pageZoomChange) {
	C.changePageZoom(window, C.int(change))
}
