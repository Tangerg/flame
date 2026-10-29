package protocol

import (
	"reflect"
	"testing"
)

func TestWireTreeRejectsAmbiguousJSONDeclarations(t *testing.T) {
	// Runtime-generated malformed declarations must not disable vet's checks
	// on actual wire declarations.
	ambiguous := reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
		{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
	})
	if err := ValidateWireTree(reflect.New(ambiguous).Elem().Interface()); err == nil {
		t.Fatal("ambiguous JSON declaration was accepted")
	}
	type recursive struct {
		ID   string     `json:"id"`
		Next *recursive `json:"next,omitzero"`
	}
	if err := ValidateWireTree(recursive{ID: "first", Next: &recursive{ID: "second"}}); err != nil {
		t.Fatalf("valid recursive declaration was refused: %v", err)
	}
}
