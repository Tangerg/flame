package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	models "github.com/Tangerg/flame/runtime/model/catalog"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestGeneratedSchedulesContractMatchesLiveMethods(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-contract"}, nil)
	registered, err := RegisterServer(server, db, sqlite.NewScheduleStore(db), workspace.Resolver{}, models.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
		return &observedRunHost{db: db, namespace: "contract-store"}, func() error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registered.Close(); err != nil {
			t.Error(err)
		}
	})
	client := sdk.NewClient(&sdk.Implementation{Name: "contract-consumer"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	owner, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "plugins", "schedules", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
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
	for _, published := range description.Methods {
		if published.Name == "flame.schedules/scan.v1" || published.Name == "flame.schedules/dispatch.v1" {
			t.Fatalf("retired host-driven method still published: %s", published.Name)
		}
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
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "schedules", required)
	if err != nil {
		t.Fatal(err)
	}
	list, err := contracts.List.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.Call(t.Context(), rpc.ListInput{}); err != nil {
		t.Fatal(err)
	}
}
