package main

import (
	"os"
	"path/filepath"
	"testing"

	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

func TestGeneratedMemoryContractMatchesLiveMethods(t *testing.T) {
	_, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	data, err := os.ReadFile(filepath.Join("..", "package", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	required, err := portable.ParseCapability(data)
	if err != nil {
		t.Fatal(err)
	}
	description, err := qimumcp.Describe(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range required.Descriptor().Methods {
		matched := false
		for _, published := range description.Methods {
			if published.Name != method.Name {
				continue
			}
			actual, err := portable.AdmitMethod(published)
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
	exported, err := qimumcp.ResolveCapability(t.Context(), session, "memory", required)
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
