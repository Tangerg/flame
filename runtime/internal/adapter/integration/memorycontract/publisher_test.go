package memorycontract_test

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/memorycontract"
)

func TestPublishedCallerPolicyHasOneSource(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "plugins", "memory", "package", "io.github.tangerg.flame", "contracts", "caller-policy.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published rpc.CallerPolicy
	if err := json.Unmarshal(data, &published, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	compiled := rpc.CallerPolicy{
		MaxModelInputBytes: memorycontract.MaxModelInputBytes, MaxModelOutputTokens: memorycontract.MaxModelOutputTokens,
		MaxModelCalls: memorycontract.MaxModelCalls, MaxEmbeddingTexts: memorycontract.MaxEmbeddingTexts,
		MaxEmbeddingInputBytes: memorycontract.MaxEmbeddingInputBytes, MaxCallbackInputBytes: memorycontract.MaxCallbackInputBytes,
		MaxCallbackOutputBytes: memorycontract.MaxCallbackOutputBytes,
	}
	if published != rpc.ModelCallerPolicy() || compiled != published {
		t.Fatalf("published/compiled caller policy drifted from its source: published=%+v compiled=%+v", published, compiled)
	}
}
