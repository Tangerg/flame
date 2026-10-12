//go:build nativeworkbenchprobe && darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework Security
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
char *workbenchSandboxHome(const char *, const char *, const char *);
bool workbenchPageReady(void *);
bool workbenchPageVisible(void *);
int workbenchPhysicalPages(void *);
void evaluateWorkbenchPage(void *, const char *, uintptr_t);
*/
import "C"

import (
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The assertion runs before DesktopHost, preferences, WebKit and journals open.
func init() {
	identity := C.CString(os.Getenv("FLAME_NATIVE_WORKBENCH_ID"))
	fixture := C.CString(os.Getenv("FLAME_NATIVE_WORKBENCH_FIXTURE"))
	canary := C.CString(os.Getenv("FLAME_NATIVE_WORKBENCH_CANARY"))
	defer C.free(unsafe.Pointer(identity))
	defer C.free(unsafe.Pointer(fixture))
	defer C.free(unsafe.Pointer(canary))
	home := C.workbenchSandboxHome(identity, fixture, canary)
	if home == nil {
		log.Fatal("native workbench: storage isolation assertion failed")
	}
	defer C.free(unsafe.Pointer(home))
	if err := os.Setenv("FLAME_HOME", filepath.Join(C.GoString(home), "flame")); err != nil {
		log.Fatal(err)
	}
}

type NativeWorkbenchGate struct{ host *DesktopHost }

func workbenchGateLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

type WorkbenchPage struct {
	ID      string `json:"id"`
	Ready   bool   `json:"ready"`
	Visible bool   `json:"visible"`
}

type WorkbenchPages struct {
	Pages         []WorkbenchPage `json:"pages"`
	MaxFrameBytes int             `json:"maxFrameBytes"`
	Physical      int             `json:"physical"`
}

type WorkbenchEvaluation struct {
	Ready bool   `json:"ready"`
	Value string `json:"value"`
}

func (g *NativeWorkbenchGate) Pages() (result WorkbenchPages) {
	result.MaxFrameBytes = maxPluginMessageBytes
	result.Pages = []WorkbenchPage{}
	application.InvokeSync(func() {
		for id, page := range g.host.pluginPages {
			result.Pages = append(result.Pages, WorkbenchPage{ID: id, Ready: bool(C.workbenchPageReady(page.pointer)), Visible: bool(C.workbenchPageVisible(page.pointer))})
		}
		result.Physical = int(C.workbenchPhysicalPages(g.host.window.NativeWindow()))
	})
	return result
}

func (g *NativeWorkbenchGate) EvaluatePage(id, script string) (WorkbenchEvaluation, error) {
	result := make(chan string, 1)
	handle := cgo.NewHandle(result)
	var found bool
	application.InvokeSync(func() {
		if page := g.host.pluginPages[id]; page != nil && bool(C.workbenchPageReady(page.pointer)) {
			found = true
			body := C.CString(script)
			defer C.free(unsafe.Pointer(body))
			C.evaluateWorkbenchPage(page.pointer, body, C.uintptr_t(handle))
		}
	})
	if !found {
		handle.Delete()
		return WorkbenchEvaluation{}, nil
	}
	select {
	case value := <-result:
		// A receipt may retire the page between enumeration and evaluation.
		// Return availability as a fact so bounded polling need not parse errors.
		var ready bool
		application.InvokeSync(func() {
			if page := g.host.pluginPages[id]; page != nil {
				ready = bool(C.workbenchPageReady(page.pointer))
			}
		})
		return WorkbenchEvaluation{Ready: ready, Value: value}, nil
	case <-time.After(10 * time.Second):
		return WorkbenchEvaluation{}, errors.New("native workbench: page evaluation timed out")
	}
}

//export receiveWorkbenchEvaluation
func receiveWorkbenchEvaluation(handle C.uintptr_t, body *C.char) {
	value := cgo.Handle(handle)
	defer value.Delete()
	value.Value().(chan string) <- C.GoString(body)
}
