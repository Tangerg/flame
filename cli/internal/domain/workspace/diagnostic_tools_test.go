package workspace

import (
	"encoding/json/jsontext"
	"testing"
)

func TestDescriptorRejectsMalformedTools(t *testing.T) {
	valid := DiagnosticToolDescriptor{Name: "inspect", Schema: jsontext.Value(`{"type":"object"}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid descriptor: %v", err)
	}
	for name, descriptor := range map[string]DiagnosticToolDescriptor{
		"empty name":  {Schema: jsontext.Value(`{}`)},
		"padded name": {Name: " inspect ", Schema: jsontext.Value(`{}`)},
		"array":       {Name: "inspect", Schema: jsontext.Value(`[]`)},
		"malformed":   {Name: "inspect", Schema: jsontext.Value(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := descriptor.Validate(); err == nil {
				t.Fatal("Validate accepted malformed descriptor")
			}
		})
	}
}

func TestInvocationRequiresConfinedJSONObject(t *testing.T) {
	valid := DiagnosticToolInvocation{Tool: DiagnosticToolDescriptor{Name: "inspect", Schema: jsontext.Value(`{}`)}, Workspace: "/repo", Arguments: jsontext.Value(`{}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid invocation: %v", err)
	}
	valid.Arguments = jsontext.Value(`null`)
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
