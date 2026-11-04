package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	framevalue "github.com/Tangerg/frame/value"

	"testing"
)

func TestContractCarriesBoundedOriginalHookMaterial(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	method, found := contract.Method("flame.hooks/evaluate.v1")
	if !found {
		t.Fatal("missing execution contract")
	}
	directory := t.TempDir()
	input := hookprovider.Command{Command: "exit 2", CWD: directory, Input: hook.Input{Event: hook.PreToolUse, CWD: directory, Tool: &hook.ToolInput{Name: "write", Arguments: `{"value":9007199254740993}`}}, TimeoutMillis: 1000}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := framevalue.EncodeJSON(hookprovider.Evaluation{Rules: []hook.Hook{{Event: input.Input.Event, Command: input.Command, Scope: hook.ScopeGlobal, Source: directory}}, Input: input.Input})
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(encoded); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"command":"exit 2","cwd":"/workspace","input":{"event":"unknown"},"timeoutMillis":1000}`,
		`{"command":"exit 2","cwd":"/workspace","timeoutMillis":1000}`,
	} {
		value, err := framevalue.ParseJSON([]byte(invalid))
		if err != nil {
			t.Fatal(err)
		}
		if method.ValidateInput(value) == nil {
			t.Fatal("incomplete or unknown event material was admitted")
		}
	}
	input.Input.CWD = "/other"
	if input.Validate() == nil {
		t.Fatal("command directory differs from the event")
	}
}
