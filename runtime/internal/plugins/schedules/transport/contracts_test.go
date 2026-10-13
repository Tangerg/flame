package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/modelcatalog"
	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
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
	if err := RegisterServer(server, db, sqlite.NewScheduleStore(db), workspace.Resolver{}, models.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
		return &observedRunHost{db: db, namespace: "contract-store"}, func() error { return nil }, nil
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := contracts.List.Call(t.Context(), session, rpc.ListInput{}); err != nil {
		t.Fatal(err)
	}
}
