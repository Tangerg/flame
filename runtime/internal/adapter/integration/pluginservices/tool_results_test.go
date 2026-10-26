package pluginservices

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestOriginalScheduleSourceAvailabilityPreservesRecoveryFailures(t *testing.T) {
	for _, state := range []string{"disabled", "disconnected", "storage failure", "canceled"} {
		t.Run(state, func(t *testing.T) {
			name := testsupport.ServerName("schedules")
			release, err := plugin.NewRelease(testsupport.Digest("original release"), plugin.Declaration{Name: "example.schedules", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "echo"}}})
			if err != nil {
				t.Fatal(err)
			}
			installation, err := plugin.New(testsupport.InstallationID(t), "original source", release)
			if err != nil {
				t.Fatal(err)
			}
			if state != "disabled" {
				if err := installation.Enable(release); err != nil {
					t.Fatal(err)
				}
			}
			store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
			source, err := installation.ServerSource(release, name)
			if err != nil {
				t.Fatal(err)
			}
			server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "echo"}
			poolSource := toolSourceRegistry{server: server}
			pool, err := mcp.Open(t.Context(), t.Context(), nil, nil, poolSource, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			schedules, err := NewToolResults(store, pool)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			}()
			ctx := t.Context()
			failure := errors.New("original store failed")
			if state == "storage failure" {
				store.failure = failure
			}
			if state == "canceled" {
				var cancel context.CancelCauseFunc
				ctx, cancel = context.WithCancelCause(ctx)
				cancel(failure)
			}
			available, err := schedules.AwaitOriginalToolSource(ctx, testsupport.MCPTool(server.ID(), "create_schedule"), release.Digest(), server.AuthorityFingerprint())
			if available {
				t.Fatal("unavailable original source became recoverable")
			}
			if state == "storage failure" || state == "canceled" {
				if !errors.Is(err, failure) {
					t.Fatalf("recovery lost the original failure: %v", err)
				}
				if state == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("recovery lost cancellation: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unavailable original source blocked ownership recovery: %v", err)
			}
		})
	}
}
