package rpc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
)

func TestGeneratedCapabilityMatchesTheActualMethodOwner(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "plugins", "memory", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := capability.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if published.Identity() != contracts.Capability.Identity() {
		t.Fatal("generated callable contract differs from actual methods; run go generate")
	}
}
