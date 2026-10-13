package plugin

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type setupTestResource struct{ closes *atomic.Int32 }

func (r setupTestResource) Close() error { r.closes.Add(1); return nil }

func TestSetupProvidesNamedImmutableBindingsAndJoinsResources(t *testing.T) {
	for _, refuses := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "refusal"}[refuses], func(t *testing.T) {
			method, err := NewMethod[struct{}, string](MethodConfig{Name: "records.read", Kind: capability.Query, Target: "workspace", Description: "Read records."})
			if err != nil {
				t.Fatal(err)
			}
			contract := testCapability(t, method.Descriptor())
			backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "setup-owner"}, nil))
			if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) { return "ready", nil }); err != nil {
				t.Fatal(err)
			}
			if err := backend.BindCapability("records", contract); err != nil {
				t.Fatal(err)
			}
			var closes, calls atomic.Int32
			var lifetime context.Context
			if err := backend.OnSetup(func(ctx context.Context, setup Setup) (io.Closer, error) {
				lifetime = ctx
				calls.Add(1)
				values, err := setup.Contributions()
				if err != nil {
					return nil, err
				}
				if len(values) != 1 || len(values[0].Requires) != 1 || values[0].Requires[0].ID != "model" || len(values[0].Requires[0].Bindings) != 1 {
					return nil, errors.New("named dependency missing")
				}
				values[0].Requires[0].Bindings[0].InstallationID = "changed"
				fresh, err := setup.Contributions()
				if err != nil {
					return nil, err
				}
				if fresh[0].Requires[0].Bindings[0].InstallationID == "changed" {
					return nil, errors.New("setup snapshot mutated")
				}
				if refuses {
					return setupTestResource{&closes}, errors.New("setup refused")
				}
				return setupTestResource{&closes}, nil
			}); err != nil {
				t.Fatal(err)
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "setup-caller"}, nil)
			if err := RegisterClient(client); err != nil {
				t.Fatal(err)
			}
			session := connectPublishedBackend(t, backend, client)
			values := []SetupContribution{{ID: "consumer", Export: "records", Requires: []SetupDependency{{ID: "model", Contract: contract.Descriptor(), Bindings: []SetupBinding{{InstallationID: "11111111-1111-4111-8111-111111111111", Digest: "1111111111111111111111111111111111111111111111111111111111111111", Contribution: "provider", Contract: contract.Descriptor()}}}}}}
			err = SetUp(t.Context(), session, values, sdk.ParamsBase{})
			if (err != nil) != refuses {
				t.Fatalf("setup refusal=%v, error=%v", refuses, err)
			}
			if err := SetUp(t.Context(), session, values, sdk.ParamsBase{}); err == nil || calls.Load() != 1 {
				t.Fatalf("setup was repeated: %v, calls=%d", err, calls.Load())
			}
			if err := backend.Close(); err != nil {
				t.Fatal(err)
			}
			if closes.Load() != 1 || lifetime == nil || lifetime.Err() == nil {
				t.Fatalf("setup lifetime survived retirement: closes=%d", closes.Load())
			}
		})
	}
}
