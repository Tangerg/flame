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
			name:  "const narrows enum",
			input: `{"type":"object","properties":{"Value":{"type":"string","enum":["first","shared"],"const":"shared"}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "pattern narrows enum",
			input: `{"type":"object","properties":{"Value":{"type":"string","enum":["first","shared"],"allOf":[{"pattern":"^shared$"}]}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "negation narrows enum",
			input: `{"type":"object","properties":{"Value":{"type":"string","enum":["first","shared"],"not":{"const":"first"}}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "conditional narrows enum",
			input: `{"type":"object","properties":{"Value":{"type":"string","enum":["first","shared"],"if":{"const":"first"},"then":false}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormChoice, options: []string{"shared"},
		},
		{
			name:  "constant text entry",
			input: `{"type":"object","properties":{"Value":{"type":"string","const":"shared"}}}`,
			paths: []string{"Value"}, kind: extension.TerminalFormText,
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
		"enum excludes const":   `{"type":"object","properties":{"Value":{"type":"string","enum":["first"],"const":"second"}}}`,
		"enum excludes pattern": `{"type":"object","properties":{"Value":{"type":"string","enum":["first"],"pattern":"^second$"}}}`,
		"enum excludes length":  `{"type":"object","properties":{"Value":{"type":"string","enum":["first"],"minLength":6}}}`,
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

func TestTerminalViewsRefuseUnsupportedOutputShapes(t *testing.T) {
	for name, output := range map[string]string{
		"nullable rows":     `{"type":"object","properties":{"Rows":{"type":["array","null"],"items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`,
		"nullable row":      `{"type":"object","properties":{"Rows":{"type":"array","items":{"type":["object","null"],"properties":{"ID":{"type":"string"}}}}}}`,
		"untyped row":       `{"type":"object","properties":{"Rows":{"type":"array","items":{"properties":{"ID":{"type":"string"}}}}}}`,
		"non-object output": `{"type":"string","properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`,
		"nullable output":   `{"type":["object","null"],"properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`,
		"union output":      `{"type":["object","string"],"properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := terminalSchemaRelease(t, `{"type":"object"}`, output); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want refusal of an unsupported output shape", err)
			}
		})
	}
}

func TestTerminalViewsKeepOptionalObjectColumns(t *testing.T) {
	output := `{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"},"Identity":{"type":["object","null"],"properties":{"Value":{"type":"string"}}}}}}}}`
	release, err := terminalSchemaRelease(t, `{"type":"object"}`, output)
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	declaration.TerminalViews[0].List.Columns[0].Field = "Identity/Value"
	if _, err := NewRelease(release.Digest(), declaration); err != nil {
		t.Fatalf("optional object column was refused: %v", err)
	}
}

func TestTerminalViewsRefuseBindingsThroughNonObjects(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		paths []string
	}{
		{name: "root", input: `{"type":"string","properties":{"Value":{"type":"string"}}}`, paths: []string{"Value"}},
		{name: "nested", input: `{"type":"object","properties":{"Settings":{"type":"string","properties":{"Value":{"type":"string"}}}}}`, paths: []string{"Settings/Value"}},
		{name: "unbound root", input: `{"type":"string"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := terminalSchemaRelease(t, test.input, terminalRowsSchema, test.paths...); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want refusal of a non-object binding path", err)
			}
		})
	}
}

func TestTerminalViewsDistinguishNullableInputAndOutput(t *testing.T) {
	input := `{"type":"object","properties":{"Value":{"type":["integer","null"]},"Cursor":{"type":["string","null"]}}}`
	for _, test := range []struct {
		name       string
		output     string
		wantRefuse bool
	}{
		{name: "nullable input with concrete output", output: `{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}},"NextCursor":{"type":"string"}}}`},
		{name: "nullable output cursor", output: `{"type":"object","properties":{"Rows":{"type":"array","items":{"type":"object","properties":{"ID":{"type":"string"}}}},"NextCursor":{"type":["string","null"]}}}`, wantRefuse: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			release, err := terminalSchemaRelease(t, input, test.output, "Value")
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			declaration.TerminalViews[0].List.Paging = &TerminalPaging{Input: "Cursor", Output: "NextCursor"}
			release, err = NewRelease(release.Digest(), declaration)
			if test.wantRefuse {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("release error = %v, want refusal of a nullable output cursor", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if form := release.TerminalViews()[0].List.Input[0].Value.Form; form.Kind != extension.TerminalFormInteger {
				t.Fatalf("nullable integer form = %+v", form)
			}
		})
	}
}

func TestTerminalViewsKeepNullableInputChoices(t *testing.T) {
	input := `{"type":"object","properties":{"Value":{"type":["string","null"],"enum":["first",null,"second"]}}}`
	release, err := terminalSchemaRelease(t, input, terminalRowsSchema, "Value")
	if err != nil {
		t.Fatal(err)
	}
	form := release.TerminalViews()[0].List.Input[0].Value.Form
	if form.Kind != extension.TerminalFormChoice || !slices.Equal(form.Options, []string{"first", "second"}) {
		t.Fatalf("nullable input choices = %+v", form)
	}
}

func TestTerminalViewsRefuseInvalidLiteralBindings(t *testing.T) {
	for _, test := range []struct {
		name, schema string
		value        any
		defaultValue bool
	}{
		{name: "type", schema: `{"type":"string"}`, value: 42},
		{name: "enum", schema: `{"type":"string","enum":["first","shared"]}`, value: "absent"},
		{name: "required object member", schema: `{"type":"object","properties":{"Name":{"type":"string"}},"required":["Name"]}`, value: map[string]any{}},
		{name: "numeric bound", schema: `{"type":"integer","minimum":1}`, value: 0},
		{name: "form default", schema: `{"type":"string","enum":["first","shared"]}`, value: "absent", defaultValue: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"type":"object","properties":{"Value":%s}}`, test.schema)
			release, err := terminalSchemaRelease(t, input, terminalRowsSchema)
			if err != nil {
				t.Fatal(err)
			}
			binding := Binding{Source: extension.TerminalBindingValue, Literal: literal(t, test.value)}
			if test.defaultValue {
				defaultBinding := binding
				binding = Binding{Source: extension.TerminalBindingForm, Form: &FormField{Title: "Value", Default: &defaultBinding}}
			}
			declaration := release.Declaration()
			declaration.TerminalViews[0].List.Input = []InputBinding{{Path: "Value", Value: binding}}
			if _, err := NewRelease(release.Digest(), declaration); !errors.Is(err, ErrInvalid) {
				t.Fatalf("release error = %v, want refusal of an invalid literal binding", err)
			}
		})
	}
}

func TestTerminalViewsKeepExactAndNullableLiteralBindings(t *testing.T) {
	const identifier int64 = 9007199254740993
	for _, test := range []struct {
		name, schema string
		value        any
	}{
		{name: "exact integer", schema: fmt.Sprintf(`{"type":"integer","minimum":%d,"maximum":%d}`, identifier, identifier), value: identifier},
		{name: "nullable value", schema: `{"type":["string","null"]}`, value: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"type":"object","properties":{"Value":%s}}`, test.schema)
			release, err := terminalSchemaRelease(t, input, terminalRowsSchema)
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			declaration.TerminalViews[0].List.Input = []InputBinding{{Path: "Value", Value: Binding{Source: extension.TerminalBindingValue, Literal: literal(t, test.value)}}}
			if _, err := NewRelease(release.Digest(), declaration); err != nil {
				t.Fatalf("valid literal binding was refused: %v", err)
			}
		})
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
