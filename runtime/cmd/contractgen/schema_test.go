package main

import (
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"testing"
)

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
