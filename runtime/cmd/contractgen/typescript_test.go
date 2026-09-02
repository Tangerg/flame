package main

import "testing"

func TestTypeScriptNestedNarrowingPreservesRequiredFieldsAndLiteralTypes(t *testing.T) {
	set := &schemaSet{defs: map[string]*schema{
		"Problem": {
			Type: schemaTypeObject,
			Properties: map[string]any{
				"detail": &schema{Type: schemaTypeString},
				"type":   &schema{Ref: refPrefix + "ProblemType"},
			},
			Required: []string{"type"},
		},
		"ProblemType": {Enum: []string{"timeout", "toolFailed"}},
	}}
	emitter := &tsEmitter{tsTypes: tsTypes{set: set}}

	got := emitter.narrow(
		&schema{Ref: refPrefix + "Problem"},
		&schema{
			Properties: map[string]any{"type": &schema{Enum: []string{"toolFailed"}}},
			Required:   []string{"detail"},
		},
	)
	const want = `{ detail: string; type: "toolFailed" }`
	if got != want {
		t.Fatalf("nested narrowing = %s, want %s", got, want)
	}
}
