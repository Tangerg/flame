package agentexec

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	agent "github.com/Tangerg/scope/agent"
)

func TestPendingToolMetadataPreservesProjectionAndRejectsForeignOwnership(t *testing.T) {
	processID, err := agent.ParseProcessID("process:metadata")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := logicalToolCallID(processID, 2, 1, "provider_reused", "write")
	if err != nil {
		t.Fatal(err)
	}
	preview := tool.StringResult("read offload ABCD")
	want := toolResultMetadata{
		MemberID:  processID.String(),
		Start:     runs.ToolCallStarted{CallID: identity.String(), SourceCallID: "provider_reused", ModelCallSequence: 2, ToolCallIndex: 1, ToolName: "write", Arguments: `{"path":"edited"}`},
		Arguments: `{"path":"edited"}`, Result: &preview, Offload: &toolresult.Ref{ID: "ABCD"},
		OutputText: "UI preview", MutatedPaths: []string{"/workspace/a", "/workspace/b"},
		Failure: &tool.Failure{Kind: tool.FailureExecution, Detail: "second write failed"},
	}
	payload, err := json.Marshal([]toolResultMetadata{want})
	if err != nil {
		t.Fatal(err)
	}
	var records []toolResultMetadata
	if err := json.Unmarshal(payload, &records); err != nil {
		t.Fatal(err)
	}
	processes := map[agent.ProcessID]struct{}{processID: {}}
	decoded, err := decodeToolMetadata(records, processes)
	if err != nil || !reflect.DeepEqual(decoded[identity.String()], want) {
		t.Fatalf("projection changed: %+v, %v", decoded, err)
	}
	records[0].MutatedPaths[0] = "changed"
	if !reflect.DeepEqual(decoded[identity.String()], want) {
		t.Fatal("decoded metadata aliases wire values")
	}
	for _, test := range []struct {
		name   string
		change func(*toolResultMetadata)
	}{
		{"foreign member", func(value *toolResultMetadata) {
			foreign, _ := agent.ParseProcessID("process:foreign")
			id, _ := logicalToolCallID(foreign, 2, 1, "provider_reused", "write")
			value.MemberID, value.Start.CallID = foreign.String(), id.String()
		}},
		{"changed call", func(value *toolResultMetadata) { value.Start.SourceCallID = "different" }},
		{"invalid offload", func(value *toolResultMetadata) { value.Offload.ID = "invalid" }},
		{"missing preview", func(value *toolResultMetadata) { value.Result = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := want.clone()
			test.change(&value)
			if _, err := decodeToolMetadata([]toolResultMetadata{value}, processes); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	if _, err := decodeToolMetadata([]toolResultMetadata{want, want}, processes); err == nil {
		t.Fatal("duplicate metadata accepted")
	}
}
