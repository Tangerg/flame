package main

/*
// Same floor as the rest of this package's darwin objects — see window_chrome_darwin.go
// for why it is stated per file.
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=10.13
#import <Cocoa/Cocoa.h>

// Read through NSUserDefaults rather than NSApp.effectiveAppearance: the window's colour
// is decided while the options struct is being built, which is before there is an
// NSApplication to ask. The key is absent in light mode and carries "Dark" in dark mode —
// there is no third value, and an unset key is the light answer.
static int systemIsDark(void) {
	NSString *style = [[NSUserDefaults standardUserDefaults] stringForKey:@"AppleInterfaceStyle"];
	return style != nil && [style rangeOfString:@"Dark"].location != NSNotFound;
}
*/
import "C"

// systemPrefersDarkAppearance reports the OS appearance the window opens against.
//
// This is NOT the app's theme. The theme is a preference the WebView owns and Go cannot
// read — but its default is "system", which resolves to exactly this, so reading the OS
// answers correctly for every user who has not chosen otherwise.
func systemPrefersDarkAppearance() bool {
	return C.systemIsDark() != 0
}
