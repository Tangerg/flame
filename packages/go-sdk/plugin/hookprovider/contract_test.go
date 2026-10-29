package hookprovider

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/qimu/jsonvalue"
)

func TestContractCarriesBoundedOriginalHookMaterial(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	method, found := contract.Method("flame.hooks/execute.v1")
	if !found {
		t.Fatal("missing execution contract")
	}
	directory := t.TempDir()
	input := Command{Command: "exit 2", CWD: directory, Input: hook.Input{Event: hook.PreToolUse, CWD: directory, Tool: &hook.ToolInput{Name: "write", Arguments: `{"value":9007199254740993}`}}, TimeoutMillis: 1000}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := jsonvalue.Encode(input)
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
		value, err := jsonvalue.Parse([]byte(invalid))
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

func TestCommandOutcomePreservesFailureWithoutInventingAnAllow(t *testing.T) {
	if (Result{}).Validate() == nil {
		t.Fatal("missing decision became success")
	}
	result := Result{ExitCode: -1, Failure: &Failure{Kind: TimedOut, Message: "deadline exceeded"}}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	method, _ := contract.Method("flame.hooks/execute.v1")
	encoded, err := jsonvalue.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateOutput(encoded.JSON()); err != nil {
		t.Fatal(err)
	}
	result.Decision = &hook.CommandDecision{Verdict: "future"}
	if result.Validate() == nil {
		t.Fatal("unknown policy advice was admitted")
	}
}
