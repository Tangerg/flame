package main

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var boundMethods = []string{"Bootstrap", "ChooseWorkingDirectory", "SaveImage", "WindowChrome"}

func TestDesktopHostBindsExactlyTheDeclaredMethods(t *testing.T) {
	hostType := reflect.TypeFor[*DesktopHost]()

	exported := make([]string, 0, hostType.NumMethod())
	for i := range hostType.NumMethod() {
		exported = append(exported, hostType.Method(i).Name)
	}
	slices.Sort(exported)

	want := slices.Clone(boundMethods)
	slices.Sort(want)
	if !slices.Equal(exported, want) {
		t.Fatalf("DesktopHost exposes %v over IPC, want exactly %v", exported, want)
	}
}

func TestWindowChromeIsUnmeasuredWithoutAWindow(t *testing.T) {
	if chrome := mustDesktopHost(t, t.TempDir()).WindowChrome(); chrome.Measured {
		t.Fatalf("WindowChrome without a window = %#v, want unmeasured", chrome)
	}
}

const hostPackage = "main"

func TestDesktopHostMethodNamesMatchTheFrontend(t *testing.T) {
	hostType := reflect.TypeFor[*DesktopHost]()

	source, err := os.ReadFile("frontend/src/rpc/desktopHost.ts")
	if err != nil {
		t.Fatalf("read the frontend's host bridge: %v", err)
	}

	for _, method := range boundMethods {
		if _, ok := hostType.MethodByName(method); !ok {
			t.Errorf("DesktopHost has no exported %s method to bind", method)
			continue
		}
		fqn := fmt.Sprintf("%s.%s.%s", hostPackage, hostType.Elem().Name(), method)
		if !strings.Contains(string(source), `"`+fqn+`"`) {
			t.Errorf("the frontend does not name %q; it cannot call what it cannot address", fqn)
		}
	}
}
