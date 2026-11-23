package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
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

// Reads the OS appearance, not the app theme. The theme defaults to "system",
// which resolves to exactly this, so it is correct for every user who has not
// chosen otherwise.
func systemPrefersDarkAppearance() bool {
	return C.systemIsDark() != 0
}
