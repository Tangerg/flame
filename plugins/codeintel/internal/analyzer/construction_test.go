package analyzer

import (
	"context"
	"testing"
)

type constructionContext struct{ context.Context }

func TestAnalyzerRequiresUsableLifetime(t *testing.T) {
	for _, lifetime := range []context.Context{nil, (*constructionContext)(nil)} {
		analyzer, err := New(lifetime, nil)
		if err == nil || analyzer != nil {
			t.Fatalf("invalid analyzer constructed: %v, %v", analyzer, err)
		}
	}
}
