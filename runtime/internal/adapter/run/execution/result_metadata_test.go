package execution

import (
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
)

func TestPendingToolMetadataPreservesProjectionAndRejectsForeignOwnership(t *testing.T) {
	processID, err := agent.ParseProcessID("process:metadata")
	if err != nil {
		t.Fatal(err)
	}
	source, err := tool.ParseRef("mcp:installation:eeb329cd-c7ce-40c9-bd90-6821fef06d30:files:write")
	if err != nil {
		t.Fatal(err)
	}
	published := source.PublishedName(true)
	identity, err := logicalToolCallID(processID, 2, 1, "provider_reused", published)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := tool.NewResult("read offload ABCD")
	if err != nil {
		t.Fatal(err)
	}
	want := toolResultMetadata{
		MemberID: processID.String(),
		Start:    runs.ToolCallStarted{CallID: identity.String(), SourceCallID: "provider_reused", ModelCallSequence: 2, ToolCallIndex: 1, ToolName: published, Source: source, Arguments: `{"path":"edited"}`},
		Result:   &preview, Offload: &transcript.ToolResultRef{ID: "ABCD"},
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
			id, _ := logicalToolCallID(foreign, 2, 1, "provider_reused", published)
			value.MemberID, value.Start.CallID = foreign.String(), id.String()
		}},
		{"changed source", func(value *toolResultMetadata) {
			value.Start.Source, _ = tool.ParseRef("mcp:user:files:write")
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
