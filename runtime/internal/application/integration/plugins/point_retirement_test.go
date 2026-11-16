package plugins

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framehost "github.com/Tangerg/frame/host"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"
)

func TestPointResolutionJoinsPreviousGeneration(t *testing.T) {
	for _, test := range []struct {
		name   string
		cancel bool
	}{{name: "released"}, {name: "canceled waiter", cancel: true}} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				contract, err := plugincontract.AdmitPoint(plugincontract.PointDescriptor{Protocol: plugincontract.Protocol, Reference: "reviews.retirement", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation, Optional: true, Ordering: plugincontract.CanonicalKeys, Value: jsontext.Value(`{"type":"string"}`), Applicability: jsontext.Value(`{"type":"object"}`)})
				if err != nil {
					t.Fatal(err)
				}
				release := func(name string) plugin.Release {
					t.Helper()
					value, err := framevalue.EncodeJSON(name)
					if err != nil {
						t.Fatal(err)
					}
					result, err := plugin.NewRelease(testsupport.Digest(name), plugin.Declaration{Name: "retirement", Points: []plugin.Point{{ID: "values", Contract: contract.Identity()}}, PointValues: []plugin.PointValue{{ID: "value", Contract: contract.Identity(), Value: value}}})
					if err != nil {
						t.Fatal(err)
					}
					return result
				}
				original, replacement := release("original"), release("replacement")
				installation, err := plugin.New(testsupport.InstallationID(t), "source", original)
				if err != nil {
					t.Fatal(err)
				}
				installation, err = installation.Enable(original)
				if err != nil {
					t.Fatal(err)
				}
				record := installation.Snapshot()
				store := &installationMemory{record: &record, catalog: releaseMemory{original.Digest(): original, replacement.Digest(): replacement}}
				admission := framehost.NewAdmission()
				host, err := framehost.New(t.Context(), framehost.Config{Authority: &frameworkTestStore{store: store, memory: store, admission: admission}})
				if err != nil {
					t.Fatal(err)
				}
				started, released := make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(released) })
				defer func() {
					unblock()
					if _, err := host.Close(t.Context()); err != nil {
						t.Error(err)
					}
				}()
				definition, err := ProjectFrameworkDeclaration(original.Declaration(), planningBackendProfile)
				if err != nil {
					t.Fatal(err)
				}
				for i, unit := range definition.Units {
					if unit.Declaration.ID != PointValuesUnit {
						continue
					}
					setup := unit.Setup
					definition.Units[i].Setup = func(ctx context.Context, instance framehost.Setup) ([]framehost.ExportValue, error) {
						if _, err := instance.Cleanup("original point values", func() error { close(started); <-released; return nil }); err != nil {
							return nil, err
						}
						return setup(ctx, instance)
					}
				}
				compiled, err := framehost.Define(framehost.Definition{Revision: original.Digest(), Units: definition.Units})
				if err != nil {
					t.Fatal(err)
				}
				if err := host.Register(definition.Contracts...); err != nil {
					t.Fatal(err)
				}
				configuration, err := EncodeFrameworkConfiguration(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := host.Restore(t.Context(), []*framehost.Plugin{compiled}); err != nil {
					t.Fatal(err)
				}
				target := plugincontract.PointContext{Target: framemcp.Target{Scope: extension.ScopeInstallation, Reference: record.ID.String()}, Attributes: jsontext.Value(`{}`)}
				if bindings, err := ResolvePoint(t.Context(), host, store, contract, target, nil); err != nil || len(bindings) != 1 || bindings[0].Digest != original.Digest() {
					t.Fatalf("original point values: %+v, %v", bindings, err)
				}
				projected, err := ProjectFrameworkRelease(replacement, planningBackendProfile)
				if err != nil {
					t.Fatal(err)
				}
				expected, found := host.Installation(record.ID.Host())
				if !found {
					t.Fatal("original installation is missing")
				}
				if err := host.Update(t.Context(), expected, framehost.InstallationConfig{ID: expected.ID, Plugin: projected.Plugin, Configuration: configuration, Enabled: true}); err != nil {
					t.Fatal(err)
				}
				<-started
				type resolution struct {
					bindings []PointBinding
					err      error
				}
				result := make(chan resolution, 1)
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				go func() {
					bindings, err := ResolvePoint(ctx, host, store, contract, target, nil)
					result <- resolution{bindings: bindings, err: err}
				}()
				synctest.Wait()
				select {
				case value := <-result:
					t.Fatalf("point resolution returned before original retirement: %+v, %v", value.bindings, value.err)
				default:
				}
				if test.cancel {
					cause := errors.New("point caller stopped")
					cancel(cause)
					synctest.Wait()
					if value := <-result; !errors.Is(value.err, context.Canceled) || !errors.Is(value.err, cause) {
						t.Fatalf("canceled retirement wait: %v", value.err)
					}
					if _, err := host.ActivateUnit(t.Context(), "", framehost.UnitRef{InstallationID: record.ID.Host(), Release: replacement.Digest(), UnitID: PointValuesUnit}); !errors.Is(err, framehost.ErrInUse) {
						t.Fatalf("caller cancellation released the original resource: %v", err)
					}
				}
				unblock()
				synctest.Wait()
				var value resolution
				if test.cancel {
					value.bindings, value.err = ResolvePoint(t.Context(), host, store, contract, target, nil)
				} else {
					value = <-result
				}
				if value.err != nil || len(value.bindings) != 1 || value.bindings[0].Digest != replacement.Digest() || string(value.bindings[0].Value.Bytes()) != `"replacement"` {
					t.Fatalf("replacement point values: %+v, %v", value.bindings, value.err)
				}
			})
		})
	}
}
