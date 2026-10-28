package jsonvalue

import (
	"encoding/json/jsontext"
	"reflect"
	"testing"
)

type recursiveEmbedding struct{ *recursiveEmbedding }
type anonymousScalar string
type scalarEmbedding struct{ anonymousScalar }

type RecursiveValue struct {
	Name     string           `json:"name"`
	Children []RecursiveValue `json:"children,omitempty"`
}

func TestTypeAdmissionRefusesUndecodableDeclarations(t *testing.T) {
	// Construct malformed declarations dynamically so vet still checks real DTOs.
	duplicate := reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
		{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
	})
	nested := reflect.StructOf([]reflect.StructField{{Name: "Child", Type: duplicate, Tag: `json:"child"`}})
	for name, typ := range map[string]reflect.Type{
		"missing type":        nil,
		"duplicate names":     duplicate,
		"invalid nested":      nested,
		"recursive embedding": reflect.TypeFor[recursiveEmbedding](),
		"anonymous scalar":    reflect.TypeFor[scalarEmbedding](),
		"non-string keys":     reflect.TypeFor[map[int]string](),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateType(typ); err == nil {
				t.Fatalf("admitted %v", typ)
			}
		})
	}
	if err := ValidateType(reflect.TypeFor[RecursiveValue]()); err != nil {
		t.Fatalf("structural recursion refused: %v", err)
	}
}

func TestDecodeValuePublishesOnlyCompleteValues(t *testing.T) {
	type input struct {
		Name string   `json:"name"`
		IDs  []string `json:"ids"`
	}
	for _, raw := range []string{
		`{"name":"changed"}`,
		`{"name":"changed","ids":null}`,
		`{"name":"changed","ids":[1]}`,
		`{"name":"changed","ids":[],"unexpected":true}`,
	} {
		value := input{Name: "original", IDs: []string{"preserved"}}
		if err := DecodeValue(jsontext.Value(raw), &value, "params"); err == nil {
			t.Fatalf("admitted %s", raw)
		}
		if !reflect.DeepEqual(value, input{Name: "original", IDs: []string{"preserved"}}) {
			t.Fatalf("refused input changed its destination: %#v", value)
		}
	}
	var value input
	if err := DecodeValue(jsontext.Value(`{"name":"accepted","ids":[]}`), &value, "params"); err != nil || value.Name != "accepted" || value.IDs == nil {
		t.Fatalf("complete value = %#v, %v", value, err)
	}
}

func TestDecodeValueRefusesInvalidDestinationsWithoutPanicking(t *testing.T) {
	for _, target := range []any{nil, 7, (*string)(nil)} {
		if err := DecodeValue(jsontext.Value(`null`), target, "params"); err == nil {
			t.Fatalf("admitted destination %T", target)
		}
	}
}

func TestOpaqueValueOwnsNullSemantics(t *testing.T) {
	var value Value
	if err := DecodeValue(jsontext.Value(`null`), &value, "payload"); err != nil || value.String() != "null" {
		t.Fatalf("opaque null = %s, %v", value.String(), err)
	}
	var typed string
	if err := DecodeValue(jsontext.Value(`null`), &typed, "name"); err == nil {
		t.Fatal("typed null admitted")
	}
}
