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
	Pages    []WorkbenchPage `json:"pages"`
	Physical int             `json:"physical"`
}

func (g *NativeWorkbenchGate) Pages() (result WorkbenchPages) {
	result.Pages = []WorkbenchPage{}
	application.InvokeSync(func() {
		for id, page := range g.host.pluginPages {
			result.Pages = append(result.Pages, WorkbenchPage{ID: id, Ready: bool(C.workbenchPageReady(page.pointer)), Visible: bool(C.workbenchPageVisible(page.pointer))})
		}
		result.Physical = int(C.workbenchPhysicalPages(g.host.window.NativeWindow()))
	})
	return result
}

func (g *NativeWorkbenchGate) EvaluatePage(id, script string) (string, error) {
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
		return "", errors.New("native workbench: page is not ready")
	}
	select {
	case value := <-result:
		return value, nil
	case <-time.After(10 * time.Second):
		return "", errors.New("native workbench: page evaluation timed out")
	}
}

//export receiveWorkbenchEvaluation
func receiveWorkbenchEvaluation(handle C.uintptr_t, body *C.char) {
	value := cgo.Handle(handle)
	defer value.Delete()
	value.Value().(chan string) <- C.GoString(body)
}
