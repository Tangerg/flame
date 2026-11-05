package mcp

import (
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

func bindRegisteredTool(t testing.TB, executable toolcontract.Tool) toolcontract.Binding {
	t.Helper()
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
