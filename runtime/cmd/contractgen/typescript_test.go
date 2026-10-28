package main

import (
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
