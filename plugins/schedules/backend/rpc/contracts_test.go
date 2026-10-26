package rpc_test

import (
	"os"
	"path/filepath"
	"testing"

	rpc "github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
)

func TestGeneratedCapabilityMatchesTheActualMethodOwner(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "package", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
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
