package mcp

import (
	"context"
	"errors"
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
		name         string
		ownerFailure error
		missing      bool
		disabled     bool
		canceled     bool
		definite     bool
		cause        error
	}{
		{name: "original connection closed", definite: true, cause: sdk.ErrConnectionClosed},
		{name: "original source disabled", disabled: true, definite: true, cause: plugin.ErrUnavailable},
		{name: "original owner unavailable", ownerFailure: storageFailure, cause: storageFailure},
		{name: "original record removed", missing: true, cause: plugin.ErrUnavailable},
		{name: "caller stopped waiting", canceled: true, cause: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			admission := frame.NewAdmission()
			var retained []plugin.Activation
			permission, err := NewCatalogPermission(admission, func(context.Context) ([]plugin.Activation, error) {
				if test.ownerFailure != nil {
					return nil, test.ownerFailure
				}
				if test.missing {
					return nil, nil
				}
				return retained, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			host, err := frame.NewHost(context.Background(), frame.HostConfig{Admission: admission, ReadAcceptedBinding: permission.Read})
			if err != nil {
				t.Fatal(err)
			}
			owner := frame.NewLifetime(context.Background())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if outcome, err := owner.Close(ctx); !outcome.Released {
					t.Errorf("original use owner did not retire: %+v, %v", outcome, err)
				}
				if outcome, err := host.Close(ctx); !outcome.Released {
					t.Errorf("original Frame instance did not retire: %+v, %v", outcome, err)
				}
			})
			definition, err := frame.Define(frame.Definition{Revision: value.Hash([]byte("retained connection")), Units: []frame.Unit{{
				Declaration: frame.UnitDeclaration{ID: "backend", Profile: frame.NativeGo},
				Setup: func(context.Context, *frame.Setup) ([]frame.ExportValue, error) {
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
			if definite, err := pool.RestorationFailure(name, original); definite != test.definite || !errors.Is(err, test.cause) {
				t.Fatalf("restore conclusion = %t, %v; want %t, %v", definite, err, test.definite, test.cause)
			}
			if err := connections.shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
