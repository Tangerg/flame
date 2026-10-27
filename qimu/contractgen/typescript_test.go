package contractgen

import "testing"

func TestTypeScriptNestedNarrowingPreservesRequiredFieldsAndLiteralTypes(t *testing.T) {
	definitions := Definitions{
		"Problem": {
			Type: SchemaTypeObject,
			Properties: map[string]any{
				"detail": &Schema{Type: SchemaTypeString},
				"type":   &Schema{Ref: RefPrefix + "ProblemType"},
			},
			Required: []string{"type"},
		},
		"ProblemType": {Enum: []string{"timeout", "toolFailed"}},
	}
	emitter := NewTypeScript(definitions)

	got := emitter.narrow(
		&Schema{Ref: RefPrefix + "Problem"},
		&Schema{
			Properties: map[string]any{"type": &Schema{Enum: []string{"toolFailed"}}},
			Required:   []string{"detail"},
		},
	)
	const want = `{ detail: string; type: "toolFailed" }`
	if got != want {
		t.Fatalf("nested narrowing = %s, want %s", got, want)
	}
}
