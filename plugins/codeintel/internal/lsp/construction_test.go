package lsp

import (
	"context"
	"testing"
)

type constructionContext struct{ context.Context }

func TestServersRequireUsableLifetime(t *testing.T) {
	for _, lifetime := range []context.Context{nil, (*constructionContext)(nil)} {
		servers, err := NewServers(lifetime, DefaultServers())
		if err == nil || servers != nil {
			t.Fatalf("invalid servers constructed: %v, %v", servers, err)
		}
	}
}
