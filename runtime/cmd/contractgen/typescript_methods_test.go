package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func TestMethodPoliciesIncludeOwnerCommittedRunStreams(t *testing.T) {
	registry := delivery.Contract()
	set := testWalkWireTypes(t, registry, dispatch.WireShapes())
	generated := newWireMethods(registry, set)
	var union string
	for line := range strings.SplitSeq(generated, "\n") {
		if strings.HasPrefix(line, "export type WireIdempotencyPolicy = ") {
			union = line
			break
		}
	}
	if !strings.Contains(union, strconv.Quote(delivery.IdempotencyOwnerRun.String())) {
		t.Fatalf("owner-committed Run stream has no generated policy type: %s", union)
	}
	for _, meta := range registry.Metas() {
		if !strings.Contains(union, strconv.Quote(meta.Idempotency.String())) {
			t.Fatalf("method %s cannot use its generated policy type", meta.Name)
		}
	}
}
