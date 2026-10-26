package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
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
	published, err := extension.ParseCapability(data)
	if err != nil {
		t.Fatal(err)
	}
	if published.Identity() != contracts.Capability.Identity() {
		t.Fatal("generated callable contract differs from actual methods; run go generate")
	}
}
