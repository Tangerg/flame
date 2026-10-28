package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/runner"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestRestorationFailureBelongsToTheExactCurrentAttempt(t *testing.T) {
	installation := testsupport.InstallationID(t)
	dependency := plugin.Dependency{InstallationID: installation, Digest: testsupport.Digest("release")}
	original := testsupport.ServerActivation("original", "contributor", dependency)
	replacement := testsupport.ServerActivation("replacement", "contributor", dependency)
	binding, err := plugins.DecodeActivation(original)
	if err != nil {
		t.Fatal(err)
	}
	name := testsupport.InstallationMCPServer(installation, "contributor")
	connections := &connections{lifetime: t.Context()}
	pool := &Pool{inner: connections}
	if definite, err := pool.RestorationFailure(name, original); definite || err != nil {
		t.Fatalf("absent catalog supplied an activation failure: definite=%t, %v", definite, err)
	}
	cause := fmt.Errorf("original backend preparation: %w", sdk.ErrConnectionClosed)
	var calls int
	attempt, err := connections.beginConnect(t.Context(), name, func(ctx context.Context, _ mcpserver.ID) (*launchPermit, error) {
		calls++
		return nil, failedRestoration(ctx, original, binding, cause)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); !errors.Is(err, cause) {
		t.Fatalf("attempt lost its original cause: %v", err)
	}
	for range 2 {
		if definite, err := pool.RestorationFailure(name, original); !definite || !errors.Is(err, cause) {
			t.Fatalf("completed original preparation was not identified: definite=%t, %v", definite, err)
		}
		if definite, err := pool.RestorationFailure(name, replacement); definite || err != nil {
			t.Fatalf("another graph inherited an original failure: definite=%t, %v", definite, err)
		}
	}
	if calls != 1 {
		t.Fatalf("recovery probe started another attempt: %d calls", calls)
	}
	successor, err := connections.beginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*launchPermit, error) {
		t.Error("a read-only recovery probe started the successor")
		return nil, errors.New("unexpected successor resolution")
	})
	if err != nil {
		t.Fatal(err)
	}
	if definite, err := pool.RestorationFailure(name, original); definite || err != nil {
		t.Fatalf("a superseded failure settled the new attempt: definite=%t, %v", definite, err)
	}
	successor.Close()
	if err := connections.shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if definite, err := pool.RestorationFailure(name, original); definite || err != nil {
		t.Fatalf("detached catalog retained an activation verdict: definite=%t, %v", definite, err)
	}
}

func TestRestorationProbePreservesUnconfirmedActivation(t *testing.T) {
	activation := testsupport.ServerActivation("original", "contributor", plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("release")})
	binding, err := plugins.DecodeActivation(activation)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		attempted qimu.BindingSnapshot
		cause     error
		outer     error
		cancel    bool
	}{
		{name: "admission read", cause: errors.New("activation read failed")},
		{name: "preparation read", attempted: binding, cause: errors.New("activation storage read failed")},
		{name: "caller cancellation", attempted: binding, cause: context.Canceled, cancel: true},
		{name: "recovery barrier", attempted: binding, cause: qimu.ErrRecoveryRequired},
		{name: "runner uncertainty", attempted: binding, cause: runner.ErrUnknown},
		{name: "unavailable and unknown", attempted: binding, cause: errors.Join(qimu.ErrUnavailable, runner.ErrUnknown)},
		{name: "stale and recovery required", attempted: binding, cause: errors.Join(qimu.ErrStaleBinding, qimu.ErrRecoveryRequired)},
		{name: "closed connection and cancellation", attempted: binding, cause: errors.Join(sdk.ErrConnectionClosed, context.Canceled)},
		{name: "outer cancellation", attempted: binding, cause: sdk.ErrConnectionClosed, outer: context.Canceled},
		{name: "outer recovery required", attempted: binding, cause: sdk.ErrConnectionClosed, outer: qimu.ErrRecoveryRequired},
		{name: "closed connection and unconfirmed release", attempted: binding, cause: errors.Join(sdk.ErrConnectionClosed, qimu.ErrUnconfirmedRelease)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.cancel {
				cancel()
			}
			name := testsupport.InstallationMCPServer(activation.Dependencies[0].InstallationID, "contributor")
			connections := &connections{lifetime: t.Context()}
			pool := &Pool{inner: connections}
			attempt, err := connections.beginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*launchPermit, error) {
				return nil, errors.Join(failedRestoration(ctx, activation, test.attempted, test.cause), test.outer)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); !errors.Is(err, test.cause) {
				t.Fatalf("attempt lost its original cause: %v", err)
			}
			definite, err := pool.RestorationFailure(name, activation)
			if definite || !errors.Is(err, test.cause) || (test.outer != nil && !errors.Is(err, test.outer)) {
				t.Fatalf("inconclusive original attempt became a definite source loss: definite=%t, %v", definite, err)
			}
			if err := connections.shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
