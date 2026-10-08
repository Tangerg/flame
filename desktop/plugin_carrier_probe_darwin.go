//go:build plugincarrierprobe && darwin

package main

/*
#include <stdbool.h>
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
static void *probeResponder(void *handle) {
 NSWindow *window=(__bridge NSWindow *)handle;
 return (__bridge void *)window.firstResponder;
}
static bool probeFocus(void *handle,double x,double y) {
 NSWindow *window=(__bridge NSWindow *)handle;
 NSView *view=[window.contentView hitTest:NSMakePoint(x,window.contentView.bounds.size.height-y)];
 while(view && ![view isKindOfClass:[WKWebView class]]) view=view.superview;
 return view && [window makeFirstResponder:view];
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func focusProbe(host *DesktopHost, x, y float64) (unsafe.Pointer, error) {
	var responder unsafe.Pointer
	var err error
	application.InvokeSync(func() {
		window := host.window.NativeWindow()
		if !bool(C.probeFocus(window, C.double(x), C.double(y))) {
			err = errors.New("native page did not accept focus")
			return
		}
		responder = C.probeResponder(window)
	})
	return responder, err
}

func verifyProbeResponder(host *DesktopHost, expected unsafe.Pointer) error {
	var err error
	application.InvokeSync(func() {
		if C.probeResponder(host.window.NativeWindow()) != expected {
			err = errors.New("closing native page did not restore workbench focus")
		}
	})
	return err
}
