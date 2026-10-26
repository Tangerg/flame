package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

func TestBoundNativeMethodsMatchTheirGeneratedContract(t *testing.T) {
	source, err := os.ReadFile("../web/src/platform/desktop.generated.ts")
	if err != nil {
		t.Fatal(err)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 {
		t.Fatalf("native service has %d source packages", len(packages))
	}
	var packageName string
	for name := range packages {
		packageName = name
	}
	service := reflect.TypeFor[*DesktopHost]()
	var actual []string
	for index := range service.NumMethod() {
		method := service.Method(index)
		if method.Name != "ServiceStartup" {
			actual = append(actual, fmt.Sprintf("%s.%s.%s", packageName, service.Elem().Name(), method.Name))
		}
	}
	var generated []string
	for _, match := range regexp.MustCompile(`(?m)^    name: "([^"]+)",$`).FindAllSubmatch(source, -1) {
		generated = append(generated, string(match[1]))
	}
	slices.Sort(actual)
	slices.Sort(generated)
	if !slices.Equal(actual, generated) {
		t.Fatalf("bound methods %v differ from generated contract %v", actual, generated)
	}
}

func TestWindowChromeIsUnmeasuredWithoutAWindow(t *testing.T) {
	if chrome := mustDesktopHost(t, t.TempDir()).WindowChrome(); chrome.Measured {
		t.Fatalf("WindowChrome without a window = %#v, want unmeasured", chrome)
	}
}
