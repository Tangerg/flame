package mcp

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/frame"
)

func testMethod[P, R any](t *testing.T, name string, kind frame.MethodKind) *Method[P, R] {
	t.Helper()
	method, err := NewMethod[P, R](frame.MethodConfig{Name: name, Kind: kind, Target: extension.ScopeInstallation, Description: name})
	if err != nil {
		t.Fatal(err)
	}
	return method
}
