package mcp

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type setupTestResource struct{ closes *atomic.Int32 }

func (r setupTestResource) Close() error { r.closes.Add(1); return nil }

func TestZeroExportBackendRetainsItsRealSetupActivationAndClose(t *testing.T) {
	backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "tool-only-owner"}, nil))
	var setups, starts, closes atomic.Int32
	if err := backend.OnSetup(func(_ context.Context, setup Setup) (io.Closer, error) {
		values, err := setup.Contributions()
		if err != nil || len(values) != 0 {
			return nil, errors.Join(errors.New("zero-export setup gained a contribution"), err)
		}
		setups.Add(1)
		return activationTestResource{&starts, &closes}, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	description, err := Describe(t.Context(), session)
	if err != nil || len(description.Methods) != 0 || len(description.Capabilities) != 0 {
		t.Fatalf("zero-export description = %+v, %v", description, err)
	}
	if err := Activate(t.Context(), session); err == nil {
		t.Fatal("zero exports bypassed setup")
	}
	if err := SetUp(t.Context(), session, []SetupContribution{}, sdk.ParamsBase{}); err != nil {
		t.Fatal(err)
	}
	if setups.Load() != 1 || starts.Load() != 0 {
		t.Fatal("setup started work or skipped its owner")
	}
	for range 2 {
		if err := Activate(t.Context(), session); err != nil {
			t.Fatal(err)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("zero-export activation started %d times", starts.Load())
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatalf("zero-export resource closed %d times", closes.Load())
	}
	if err := Activate(t.Context(), session); err == nil {
		t.Fatal("retired zero-export instance restarted")
	}
}

func TestSetupProvidesNamedImmutableBindingsAndJoinsResources(t *testing.T) {
	for _, refuses := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "refusal"}[refuses], func(t *testing.T) {
			method, err := NewMethod[struct{}, string](MethodConfig{Name: "records.read", Kind: portable.Query, Target: "workspace", Description: "Read records."})
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

type activationTestResource struct{ starts, closes *atomic.Int32 }

func (r activationTestResource) Start() error { r.starts.Add(1); return nil }
func (r activationTestResource) Close() error { r.closes.Add(1); return nil }
func TestActivationStartsOnceOnlyAfterSuccessfulSetup(t *testing.T) {
	method, err := NewMethod[struct{}, string](MethodConfig{Name: "background.read", Kind: portable.Query, Target: "installation", Description: "Read background records."})
	if err != nil {
		t.Fatal(err)
	}
	backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "background"}, nil))
	if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) { return "ready", nil }); err != nil {
		t.Fatal(err)
	}
	contract := testCapability(t, method.Descriptor())
	if err := backend.BindCapability("background", contract); err != nil {
		t.Fatal(err)
	}
	var starts, closes atomic.Int32
	if err := backend.OnSetup(func(context.Context, Setup) (io.Closer, error) { return activationTestResource{&starts, &closes}, nil }); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	if err := Activate(t.Context(), session); err == nil {
		t.Fatal("activation bypassed setup")
	}
	values := []SetupContribution{{ID: "background", Export: "background", Requires: []SetupDependency{}, Background: &portable.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}}}
	if err := SetUp(t.Context(), session, values, sdk.ParamsBase{}); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 {
		t.Fatal("setup started business work")
	}
	for range 2 {
		if err := Activate(t.Context(), session); err != nil {
			t.Fatal(err)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("activation starts=%d", starts.Load())
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatalf("activation closes=%d", closes.Load())
	}
	if err := Activate(t.Context(), session); err == nil {
		t.Fatal("retired activation restarted")
	}
}
