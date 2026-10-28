package mcp

import (
	"context"
	"testing"
)

func TestOpenRejectsMissingSourceRegistry(t *testing.T) {
	for _, registry := range []sourceRegistry{nil, (*mutableSourceRegistry)(nil)} {
		pool, err := Open(t.Context(), t.Context(), nil, nil, registry, nil, nil, nil, nil, nil)
		if pool != nil {
			if closeErr := pool.Shutdown(context.WithoutCancel(t.Context())); closeErr != nil {
				t.Error(closeErr)
			}
		}
		if err == nil || pool != nil {
			t.Fatalf("invalid connection pool opened: %v, %v", pool, err)
		}
	}
}
