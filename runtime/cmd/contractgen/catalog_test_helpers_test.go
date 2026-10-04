package main

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func testWalkWireTypes(t *testing.T, registry *delivery.Registry, shapes *dispatch.Shapes) *schemaSet {
	t.Helper()
	set, err := walkWireTypes(registry, shapes)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func testBuild(t *testing.T, set *schemaSet) manifest {
	t.Helper()
	built, err := build(set)
	if err != nil {
		t.Fatal(err)
	}
	return built
}
