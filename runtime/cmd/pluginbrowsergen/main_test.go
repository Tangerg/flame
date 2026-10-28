package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestPureProtocolIsTheCompiledControlProjection(t *testing.T) {
	value, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	existing, err := os.ReadFile("../../../packages/go-sdk/protocol/plugin_browser.generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value, existing) {
		t.Fatal("browser public protocol projection drifted; run make generate")
	}
	file, err := parser.ParseFile(token.NewFileSet(), "plugin_browser.generated.go", value, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Imports) != 0 {
		t.Fatal("public browser protocol acquired an execution dependency")
	}
}
