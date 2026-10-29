package main

import (
	"reflect"
	"testing"

	"github.com/Tangerg/flame/qimu/contractgen"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

type recursiveWireObject struct {
	Children []recursiveWireObject `json:"children,omitempty"`
}

type recursiveWireCollection []recursiveWireCollection
type recursiveWireMap map[string]recursiveWireMap
type recursiveAnonymousWireCollection []struct {
	Children recursiveAnonymousWireCollection `json:"children,omitempty"`
}

func TestWireGraphPreservesEmbeddedPaginationOwner(t *testing.T) {
	set, err := walkWireTypes(delivery.Contract(), dispatch.WireShapes())
	if err != nil {
		t.Fatal(err)
	}
	page := set.defs["PageQuery"]
	if page == nil || page.Properties["limit"] == nil || page.Properties["cursor"] == nil {
		t.Fatal("embedded public pagination owner disappeared from the wire graph")
	}
	sessions := set.defs["ListSessionsRequest"]
	if sessions == nil || sessions.Properties["limit"] == nil || sessions.Properties["cursor"] == nil {
		t.Fatal("pagination owner replaced flattened request fields")
	}
}

func TestWireGraphPreservesNamedObjectRecursion(t *testing.T) {
	set := newSchemaSet(&dispatch.Shapes{})
	node := set.walk(reflect.TypeFor[[]recursiveWireObject]())
	want := contractgen.RefPrefix + "recursiveWireObject"
	if node.Items == nil || node.Items.Ref != want {
		t.Fatalf("root items = %#v, want %s", node.Items, want)
	}
	owner := set.defs["recursiveWireObject"]
	if owner == nil {
		t.Fatal("recursive object definition is missing")
	}
	children, ok := owner.Properties["children"].(*contractgen.Schema)
	if !ok || children.Items == nil || children.Items.Ref != want {
		t.Fatalf("recursive children = %#v, want %s", children, want)
	}
}

func TestWireGraphRefusesCollectionOnlyRecursion(t *testing.T) {
	for _, root := range []reflect.Type{
		reflect.TypeFor[recursiveWireCollection](),
		reflect.TypeFor[recursiveWireMap](),
		reflect.TypeFor[recursiveAnonymousWireCollection](),
	} {
		t.Run(root.Name(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("collection recursion without a named object reference was admitted")
				}
			}()
			newSchemaSet(&dispatch.Shapes{}).walk(root)
		})
	}
}
