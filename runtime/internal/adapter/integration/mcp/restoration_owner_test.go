package mcp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestRetainedRestorationReadsOriginalOwnerThroughFrame(t *testing.T) {
	storageFailure := errors.New("original owner cannot be read")
	for _, test := range []struct {
		name          string
		recordFailure error
		setupFailure  error
		closedChannel bool
		hostCloses    bool
		missing       bool
		disabled      bool
		canceled      bool
		definite      bool
		cause         error
	}{
		{name: "original connection closed", definite: true, cause: sdk.ErrConnectionClosed},
		{name: "original Frame channel closed", closedChannel: true, definite: true, cause: sdk.ErrConnectionClosed},
		{name: "preparation storage owner closed", setupFailure: frame.ErrClosed, cause: frame.ErrClosed},
		{name: "Frame host closed during preparation", setupFailure: frame.ErrClosed, hostCloses: true, cause: frame.ErrClosed},
		{name: "original source disabled", disabled: true, definite: true, cause: plugin.ErrUnavailable},
		{name: "original business owner unavailable", recordFailure: storageFailure, cause: storageFailure},
		{name: "original business owner closed", recordFailure: frame.ErrClosed, cause: frame.ErrClosed},
		{name: "original record settled", missing: true},
		{name: "caller stopped waiting", canceled: true, cause: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			admission := frame.NewAdmission()
			var retained []plugin.Activation
			permission, err := NewCatalogPermission(admission, func(context.Context) ([]plugin.Activation, error) {
				if test.recordFailure != nil {
					return nil, test.recordFailure
				}
				if test.missing {
					return nil, nil
				}
				return retained, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			owner := frame.NewLifetime(context.Background())
			host, err := frame.NewHost(context.Background(), frame.HostConfig{Lifetime: owner, Admission: admission, ReadAcceptedBinding: permission.Read})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if outcome, err := host.Close(ctx); !outcome.Released {
					t.Errorf("original Frame instance did not retire: %+v, %v", outcome, err)
				}
			})
			definition, err := frame.Define(frame.Definition{Revision: value.Hash([]byte("retained connection")), Units: []frame.Unit{{
				Declaration: frame.UnitDeclaration{ID: "backend", Profile: frame.NativeGo},
				Setup: func(ctx context.Context, _ *frame.Setup) ([]frame.ExportValue, error) {
					if test.hostCloses {
						owner.BeginClose(frame.ErrClosed)
						<-owner.Context().Done()
					}
					if test.setupFailure != nil {
						return nil, test.setupFailure
					}
					if test.closedChannel {
						reader, _ := io.Pipe()
						channel := closedFrameChannel{reader}
						defer channel.Close()
						connection := &processConnection{maximum: sdk.DefaultMaxLineLength}
						return connection.Connect(ctx, channel)
					}
					return nil, sdk.ErrConnectionClosed
				},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			installation := testsupport.InstallationID(t)
			record, err := host.Install(t.Context(), frame.InstallationConfig{ID: installation.Host(), Plugin: definition, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := host.PlanUnit(t.Context(), "", record.Unit("backend"), nil)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := plan.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			original, err := plugins.ProjectActivation(binding)
			if err != nil {
				t.Fatal(err)
			}
			retained = []plugin.Activation{original}
			grant, err := permission.grant(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if test.disabled {
				if err := host.SetEnabled(t.Context(), record.ID, false); err != nil {
					t.Fatal(err)
				}
			}
			installed := &Installed{host: host, owner: owner, permission: permission}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.canceled {
				cancel()
			}
			if test.missing {
				if err := installed.restoreRetained(ctx, original, grant); err != nil {
					t.Fatalf("settled original work blocked the catalog: %v", err)
				}
				if _, err := host.AcquireRecord(ctx, original.Identity.String(), frame.UseConfig{Owner: owner, Scope: grant}); !errors.Is(err, frame.ErrUnavailable) {
					t.Fatalf("settled original record authorized execution: %v", err)
				}
				if _, err := host.Acquire(ctx, plan, frame.UseConfig{Owner: owner, Scope: grant}); !errors.Is(err, sdk.ErrConnectionClosed) {
					t.Fatalf("fresh catalog did not reach its current source: %v", err)
				}
				return
			}
			name := testsupport.InstallationMCPServer(installation, "contributor")
			connections := &connections{lifetime: t.Context()}
			pool := &Pool{inner: connections}
			attempt, err := connections.beginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*launchPermit, error) {
				return nil, installed.restoreRetained(ctx, original, grant)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); !errors.Is(err, test.cause) {
				t.Fatalf("restore lost original failure: %v; want %v", err, test.cause)
			}
			if test.closedChannel && (ctx.Err() != nil || owner.Context().Err() != nil) {
				t.Fatal("a refused channel write ended an original acquisition owner")
			}
			if definite, err := pool.RestorationFailure(name, original); definite != test.definite || !errors.Is(err, test.cause) {
				t.Fatalf("restore conclusion = %t, %v; want %t, %v", definite, err, test.definite, test.cause)
			}
			if test.hostCloses {
				if _, err := pool.RestorationFailure(name, original); !errors.Is(err, context.Canceled) {
					t.Fatalf("closing original owner lost its cancellation: %v", err)
				}
			}
			if err := connections.shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type closedFrameChannel struct{ *io.PipeReader }

func (closedFrameChannel) Write([]byte) (int, error) { return 0, frame.ErrClosed }
