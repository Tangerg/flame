package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	"github.com/Tangerg/flame/qimu/portable"
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
	published, err := portable.ParseCapability(data)
	if err != nil {
		t.Fatal(err)
	}
	if published.Identity() != contracts.Capability.Identity() {
		t.Fatal("generated callable contract differs from actual methods; run go generate")
	}
}
