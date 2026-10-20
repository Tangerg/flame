package transport_test

import (
	"os"
	"path/filepath"
	"testing"

	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	rpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

func TestGeneratedMemoryContractMatchesLiveMethods(t *testing.T) {
	_, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	data, err := os.ReadFile(filepath.Join("..", "..", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	required, err := capability.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	description, err := pluginsdk.Describe(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range required.Descriptor().Methods {
		matched := false
		for _, published := range description.Methods {
			if published.Name != method.Name {
				continue
			}
			actual, err := capability.AdmitMethod(published)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := required.Method(method.Name)
			if actual.Identity() != expected.Identity() {
				t.Fatalf("live method %s differs from generated contract", method.Name)
			}
			matched = true
		}
		if !matched {
			t.Fatalf("required method %s missing", method.Name)
		}
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "memory", required)
	if err != nil {
		t.Fatal(err)
	}
	list, err := contracts.List.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	page, err := list.Call(t.Context(), rpc.ListInput{Scope: memory.ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if page.Capacity != memory.MaxVisiblePerTarget {
		t.Fatalf("live page capacity = %d", page.Capacity)
	}
}
