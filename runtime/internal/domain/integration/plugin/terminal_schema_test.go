package plugin

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/frame/portable"
)

const terminalRowsSchema = `{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`

func terminalSchemaRelease(t *testing.T, input, output string, paths ...string) (Release, error) {
	t.Helper()
	contract, err := plugincontract.NewCapability(
		plugincontract.CapabilityConfig{Reference: "records", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation},
		portable.MethodDescriptor{Name: "records/list", Kind: portable.Query, Target: extension.ScopeInstallation, Input: jsontext.Value(input), Output: jsontext.Value(output)},
	)
	if err != nil {
		t.Fatal(err)
	}
	bindings := make([]InputBinding, len(paths))
	for index, path := range paths {
		bindings[index] = InputBinding{Path: path, Value: Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: path}}}
	}
	return NewRelease(testDigest("terminal-schema"), Declaration{
		Name:          "terminal-schema",
		Servers:       []Server{{Name: serverName("backend"), Startup: DeclaredConnection{Connection: mcpserver.StdioConnection{Command: "backend"}}}},
		Contributions: []Contribution{{ID: "records-provider", Server: serverName("backend"), Export: "records", Contract: contract.Identity()}},
		TerminalViews: []TerminalView{{ID: "records", Title: "Records", Contribution: "records-provider", List: TerminalList{
			Method: "records/list", Rows: "Rows", Key: "ID", Input: bindings,
			Columns: []TerminalColumn{{Title: "ID", Field: "ID"}},
		}}},
	})
}

func TestTerminalViewsRefuseUnboundConjunctiveRequirements(t *testing.T) {
	for name, input := range map[string]string{
		"reference sibling": `{"$defs":{"base":{"type":"object","properties":{"A":{"type":"string"},"B":{"type":"string"}}}},"$ref":"#/$defs/base","required":["B"]}`,
		"allOf members":     `{"type":"object","properties":{"A":{"type":"string"},"B":{"type":"string"}},"allOf":[{"required":["A"]},{"required":["B"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := terminalSchemaRelease(t, input, terminalRowsSchema, "A"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want refusal of unbound required input B", err)
			}
		})
	}
	input := `{"type":"object","allOf":[{"properties":{"Settings":{"type":"object","properties":{"A":{"type":"string"}},"required":["A"]}}},{"properties":{"Settings":{"type":"object","properties":{"B":{"type":"string"}},"required":["B"]}}}]}`
	if _, err := terminalSchemaRelease(t, input, terminalRowsSchema, "Settings/A"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("release error = %v, want refusal of unbound required input Settings/B", err)
	}
}

func TestTerminalViewsProjectConjunctiveFormShapes(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		paths   []string
		kind    extension.TerminalFormKind
		options []string
	}{
		{
			name:  "properties across allOf members",
			input: `{"type":"object","allOf":[{"properties":{"A":{"type":"string"}},"required":["A"]},{"properties":{"B":{"type":"string"}},"required":["B"]}]}`,
			paths: []string{"A", "B"}, kind: extension.TerminalFormText,
		},
		{
			name:  "enum reference sibling",
			input: `{"type":"object","properties":{"Value":{"$ref":"#/$defs/choice","enum":["shared"]}},"$defs":{"choice":{"type":"string","enum":["first","shared"]}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "enum across property conjuncts",
			input: `{"type":"object","properties":{"Value":{"type":"string","enum":["first","shared"]}},"allOf":[{"properties":{"Value":{"enum":["shared","second"]}}}]}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "integer narrows number",
			input: `{"type":"object","properties":{"Value":{"type":"number","allOf":[{"type":"integer"}]}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormInteger,
		},
		{
			name:  "nested local reference",
			input: `{"type":"object","properties":{"Value":{"$ref":"#/$defs/holder/properties/Value"}},"$defs":{"holder":{"type":"object","properties":{"Value":{"type":"string"}}}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormText,
		},
		{
			name: "complete admitted reference chain", input: terminalReferenceChain(t),
			paths: []string{"Value"}, kind: extension.TerminalFormText,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			release, err := terminalSchemaRelease(t, test.input, terminalRowsSchema, test.paths...)
			if err != nil {
				t.Fatal(err)
			}
			for _, binding := range release.TerminalViews()[0].List.Input {
				form := binding.Value.Form
				if form.Kind != test.kind || !slices.Equal(form.Options, test.options) {
					t.Errorf("form %q = %+v, want kind %q and options %v", binding.Path, form, test.kind, test.options)
				}
			}
		})
	}
}

func TestTerminalViewsRefuseUnrepresentableConjunctions(t *testing.T) {
	for name, input := range map[string]string{
		"disjoint enum choices": `{"type":"object","properties":{"Value":{"enum":["first"],"allOf":[{"enum":["second"]}]}}}`,
		"incompatible types":    `{"type":"object","properties":{"Value":{"type":"number","enum":["first"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := terminalSchemaRelease(t, input, terminalRowsSchema, "Value"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want refusal of a form without a valid choice", err)
			}
		})
	}
}

func TestTerminalViewsRequireArrayRows(t *testing.T) {
	output := `{"type":"object","properties":{"Rows":{"type":"object","items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`
	if _, err := terminalSchemaRelease(t, `{"type":"object"}`, output); !errors.Is(err, ErrInvalid) {
		t.Fatalf("release error = %v, want refusal of object rows", err)
	}
}

func terminalReferenceChain(t *testing.T) string {
	t.Helper()
	depth := portable.ProfileLimits().MaxReferenceDepth - 1
	definitions := make(map[string]jsontext.Value, depth)
	for index := range depth {
		value := `{"type":"object","properties":{"Value":{"type":"string"}}}`
		if index+1 < depth {
			value = fmt.Sprintf(`{"$ref":"#/$defs/d%d"}`, index+1)
		}
		definitions[fmt.Sprintf("d%d", index)] = jsontext.Value(value)
	}
	input, err := json.Marshal(struct {
		Definitions map[string]jsontext.Value `json:"$defs"`
		Reference   string                    `json:"$ref"`
	}{Definitions: definitions, Reference: "#/$defs/d0"})
	if err != nil {
		t.Fatal(err)
	}
	return string(input)
}
