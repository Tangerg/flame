package delivery

import (
	"context"
	"testing"
)

type constructionContext struct{ context.Context }

func TestEndpointRejectsTypedNilCollaborators(t *testing.T) {
	for _, config := range []EndpointConfig{
		{Lifetime: t.Context(), IdempotencyStore: (*memoryIdempotencyStore)(nil)},
		{Lifetime: (*constructionContext)(nil)},
	} {
		endpoint, err := NewEndpoint(struct{}{}, config)
		if err == nil || endpoint != nil {
			t.Fatalf("invalid endpoint constructed: %v, %v", endpoint, err)
		}
	}
}
