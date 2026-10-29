package main

import (
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu/contractgen"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

type unconditionalRequiredShape struct {
	CreatedAt time.Time `json:"createdAt,omitzero"`
}

func TestTypeScriptPublishesUnconditionalRequirements(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeFor[unconditionalRequiredShape]()
	set := &schemaSet{
		defs:   make(map[string]*contractgen.Schema),
		origin: make(map[string]reflect.Type),
		enums:  make(map[reflect.Type][]string),
		unions: make(map[reflect.Type]dispatch.UnionSpec),
		constraints: map[reflect.Type][]dispatch.ConditionalRule{
			shape: {{Required: []string{"createdAt"}}},
		},
		values: make(map[reflect.Type][]dispatch.FieldConstraint),
	}
	body := contractgen.NewTypeScript(nil).ObjectBody(set.object(shape, nil), "")
	if strings.Contains(body, "createdAt?") || !strings.Contains(body, "createdAt: string;") {
		t.Fatalf("TypeScript body = %q, want required createdAt", body)
	}
}

func TestTypeScriptValidatorsPreserveStringValues(t *testing.T) {
	for _, value := range []string{"\a", "\U000e0001", "\u2028", `\d+`} {
		emitter := &checkEmitter{used: make(map[string]bool)}
		for _, generation := range []struct {
			name string
			call string
		}{
			{name: "literal", call: emitter.compile(&contractgen.Schema{Const: value})},
			{name: "pattern", call: emitter.stringChecks(&contractgen.Schema{Pattern: value})[0]},
		} {
			literal, ok := strings.CutPrefix(generation.call, generation.name+"(")
			if !ok || !strings.HasSuffix(literal, ")") {
				t.Fatalf("unexpected generated validator: %s", generation.call)
			}
			var decoded string
			if err := json.Unmarshal([]byte(strings.TrimSuffix(literal, ")")), &decoded); err != nil {
				t.Fatalf("generated validator contains an invalid string: %s: %v", generation.call, err)
			}
			if decoded != value {
				t.Fatalf("generated validator changed %q into %q: %s", value, decoded, generation.call)
			}
		}
	}
}
