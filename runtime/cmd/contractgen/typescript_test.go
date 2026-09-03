package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

type unconditionalRequiredShape struct {
	CreatedAt time.Time `json:"createdAt,omitzero"`
}

func TestTypeScriptPublishesUnconditionalRequirements(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeFor[unconditionalRequiredShape]()
	set := &schemaSet{
		defs:   make(map[string]*schema),
		origin: make(map[string]reflect.Type),
		enums:  make(map[reflect.Type][]string),
		unions: make(map[reflect.Type]dispatch.UnionSpec),
		constraints: map[reflect.Type][]dispatch.ConditionalRule{
			shape: {{Required: []string{"createdAt"}}},
		},
		values: make(map[reflect.Type][]dispatch.FieldConstraint),
	}
	body := (&tsEmitter{}).objectBody(set.object(shape), "")
	if strings.Contains(body, "createdAt?") || !strings.Contains(body, "createdAt: string;") {
		t.Fatalf("TypeScript body = %q, want required createdAt", body)
	}
}

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
