package modelclient

import (
	"strings"
	"testing"
)

func TestEmbeddingSpaceIDIncludesEndpointIdentityWithoutPersistingIt(t *testing.T) {
	const (
		provider  = "openai-compatible"
		model     = "text-embedding"
		firstURL  = "https://first.example.test/v1"
		secondURL = "https://second.example.test/v1"
	)

	first := embeddingSpaceID(provider, model, firstURL)
	if first == embeddingSpaceID(provider, model, secondURL) {
		t.Fatal("embedding space did not change with the endpoint")
	}
	if first != embeddingSpaceID(provider, model, firstURL) {
		t.Fatal("embedding space is not deterministic")
	}
	if strings.Contains(first, firstURL) {
		t.Fatal("embedding space persisted the raw endpoint")
	}
}

func TestEmbeddingSpaceIDPreservesFieldBoundaries(t *testing.T) {
	left := embeddingSpaceID("provider", "model-part", "endpoint")
	right := embeddingSpaceID("provider-model", "part", "endpoint")
	if left == right {
		t.Fatal("different embedding identity fields produced the same space")
	}
	if !strings.HasPrefix(left, embeddingSpaceVersion) {
		t.Fatalf("embedding space %q does not carry its encoding version", left)
	}
}
