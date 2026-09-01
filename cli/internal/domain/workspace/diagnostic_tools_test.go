package workspace

import (
	"encoding/json"
	"testing"
)

func TestDescriptorRejectsMalformedTools(t *testing.T) {
	valid := DiagnosticToolDescriptor{Name: "inspect", Schema: json.RawMessage(`{"type":"object"}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid descriptor: %v", err)
	}
	for name, descriptor := range map[string]DiagnosticToolDescriptor{
		"empty name": {Schema: json.RawMessage(`{}`)},
		"array":      {Name: "inspect", Schema: json.RawMessage(`[]`)},
		"malformed":  {Name: "inspect", Schema: json.RawMessage(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := descriptor.Validate(); err == nil {
				t.Fatal("Validate accepted malformed descriptor")
			}
		})
	}
}

func TestInvocationRequiresConfinedJSONObject(t *testing.T) {
	valid := DiagnosticToolInvocation{Tool: DiagnosticToolDescriptor{Name: "inspect", Schema: json.RawMessage(`{}`)}, Workspace: "/repo", Arguments: json.RawMessage(`{}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid invocation: %v", err)
	}
	valid.Arguments = json.RawMessage(`null`)
	if err := valid.Validate(); err == nil {
		t.Fatal("Validate accepted non-object arguments")
	}
}

func TestParseArgumentsDefaultsAndRejectsNonObjects(t *testing.T) {
	arguments, err := ParseDiagnosticToolArguments("")
	if err != nil || string(arguments) != `{}` {
		t.Fatalf("ParseArguments empty = (%s, %v)", arguments, err)
	}
	if _, err := ParseDiagnosticToolArguments(`[]`); err == nil {
		t.Fatal("ParseArguments accepted an array")
	}
}
