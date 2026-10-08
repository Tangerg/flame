package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
void *createIsolatedCarrier(void *window, const char *url, bool lockdown, uintptr_t handle);
void closeIsolatedCarrier(void *carrier);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

type isolatedCarrier struct {
	native unsafe.Pointer
	handle cgo.Handle
}

// Creation and close run on Wails' main thread; the native frame owns sender identity.
func newIsolatedCarrier(window unsafe.Pointer, entry string, lockdown bool, complete func(string, error, *nativeIsolation)) (*isolatedCarrier, error) {
	if window == nil {
		return nil, errors.New("native carrier requires a live window")
	}
	handle := cgo.NewHandle(complete)
	url := C.CString(entry)
	defer C.free(unsafe.Pointer(url))
	native := C.createIsolatedCarrier(window, url, C.bool(lockdown), C.uintptr_t(handle))
	if native == nil {
		handle.Delete()
		return nil, errors.New("isolated WebKit carrier unavailable")
	}
	return &isolatedCarrier{native: native, handle: handle}, nil
}

func (c *isolatedCarrier) Close() {
	if c.native == nil {
		return
	}
	C.closeIsolatedCarrier(c.native)
	c.native = nil
	c.handle.Delete()
}

//export completeIsolatedProbe
func completeIsolatedProbe(handle C.uintptr_t, payload *C.char, failed C.bool, lockdown C.bool, rejected C.uint64_t) {
	complete := cgo.Handle(handle).Value().(func(string, error, *nativeIsolation))
	value := C.GoString(payload)
	var cause error
	if bool(failed) {
		cause = errors.New(value)
		value = ""
	}
	complete(value, cause, &nativeIsolation{LockdownEnabled: bool(lockdown), RejectedMessages: uint64(rejected)})
}
