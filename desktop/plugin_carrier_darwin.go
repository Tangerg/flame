package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
void *createPluginPage(void *, const char *, uintptr_t, size_t);
void closePluginPage(void *);
void sendPluginPage(void *, const char *);
void positionPluginPage(void *, double, double, double, double);
*/
import "C"
import (
	"encoding/json"
	"errors"
	"runtime/cgo"
	"unsafe"
)

type nativePluginPage struct {
	pointer unsafe.Pointer
	handle  cgo.Handle
}

// Callers serialize creation, publication and retirement on the native main thread.
func createNativePluginPage(window unsafe.Pointer, html string, bounds PluginPageBounds, receive func(json.RawMessage)) (*nativePluginPage, error) {
	handle := cgo.NewHandle(receive)
	body := C.CString(html)
	defer C.free(unsafe.Pointer(body))
	pointer := C.createPluginPage(window, body, C.uintptr_t(handle), C.size_t(maxPluginMessageBytes))
	if pointer == nil {
		handle.Delete()
		return nil, errors.New("plugin carrier: isolated WebKit unavailable")
	}
	page := &nativePluginPage{pointer: pointer, handle: handle}
	page.position(bounds)
	return page, nil
}
func (p *nativePluginPage) close() {
	if p.pointer == nil {
		return
	}
	C.closePluginPage(p.pointer)
	p.pointer = nil
	p.handle.Delete()
}
func (p *nativePluginPage) send(message string) {
	body := C.CString(message)
	defer C.free(unsafe.Pointer(body))
	C.sendPluginPage(p.pointer, body)
}
func (p *nativePluginPage) position(b PluginPageBounds) {
	C.positionPluginPage(p.pointer, C.double(b.X), C.double(b.Y), C.double(b.Width), C.double(b.Height))
}

//export receivePluginPage
func receivePluginPage(handle C.uintptr_t, body *C.char) {
	value := C.GoString(body)
	if len(value) > maxPluginMessageBytes || !json.Valid([]byte(value)) {
		value = `{"type":"failure","reason":"Invalid native carrier message."}`
	}
	cgo.Handle(handle).Value().(func(json.RawMessage))(json.RawMessage(value))
}
