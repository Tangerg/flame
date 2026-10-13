package rpc_test

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
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

func TestPublisherSnapshotOwnsIdentityAndContentAdmission(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	method, ok := contracts.Capability.Method(rpc.Update)
	if !ok {
		t.Fatal("update contract is absent")
	}
	valid := memory.ItemSnapshot{ID: memory.NewItemID([memory.ItemIDEntropyBytes]byte{1}), Scope: memory.ScopeUser, Content: strings.Repeat("界", memory.MaxContentCharacters), Origin: memory.OriginUser, Status: memory.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(), Embedding: []float32{}}
	encode := func(row memory.ItemSnapshot) []byte {
		t.Helper()
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if err := method.ValidateOutput(encode(valid)); err != nil {
		t.Fatalf("publisher rejected Unicode boundary: %v", err)
	}
	for _, content := range []string{" ", valid.Content + "界"} {
		row := valid
		row.Content = content
		if err := method.ValidateOutput(encode(row)); err == nil {
			t.Fatal("publisher accepted invalid content")
		}
	}
	data := strings.Replace(string(encode(valid)), valid.ID.String(), "mem_1", 1)
	if err := method.ValidateOutput([]byte(data)); err == nil {
		t.Fatal("publisher accepted malformed identity")
	}
}

func TestPublishedPageCapacityDerivesFromTheBusinessOwner(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	method, _ := contracts.Capability.Method(rpc.List)
	page := rpc.Page{Rows: []memory.ItemSnapshot{}, Capacity: memory.MaxVisiblePerTarget}
	data, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateOutput(data); err != nil {
		t.Fatalf("owner capacity rejected: %v", err)
	}
	page.Capacity++
	data, err = json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateOutput(data); err == nil {
		t.Fatal("a competing capacity was admitted by the publisher contract")
	}
	for _, size := range []int{0, memory.MaxManagementPageItems + 1} {
		request, err := jsoninput.Encode(rpc.ListInput{Scope: memory.ScopeUser, Limit: &size})
		if err != nil {
			t.Fatal(err)
		}
		if err := method.ValidateInput(request); err == nil {
			t.Fatalf("publisher accepted invalid page size %d", size)
		}
	}
}
