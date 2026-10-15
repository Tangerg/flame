package transport

import (
	"context"
	"testing"
)

func TestNestedRequestReplacesTransportMetadataIncludingAbsentHeaders(t *testing.T) {
	for _, header := range []struct {
		name string
		with func(context.Context, string) context.Context
		read func(context.Context) string
	}{
		{"last-event-id", WithLastEventID, LastEventIDFrom},
		{"idempotency-key", WithIdempotencyKey, IdempotencyKeyFrom},
		{"idempotency-namespace", WithIdempotencyNamespace, IdempotencyNamespaceFrom},
	} {
		t.Run(header.name, func(t *testing.T) {
			parent := header.with(t.Context(), "original")
			for _, value := range []string{"", "nested"} {
				child := header.with(parent, value)
				if got := header.read(child); got != value {
					t.Fatalf("nested request inherited transport metadata: got %q, want %q", got, value)
				}
			}
			if got := header.read(parent); got != "original" {
				t.Fatalf("nested request changed its calling owner: %q", got)
			}
		})
	}
}
