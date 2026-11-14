package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/process/runner"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestRestorationFailureBelongsToTheExactCurrentAttempt(t *testing.T) {
	installed := &Installed{owner: frame.NewLifetime(t.Context())}
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
		return nil, installed.failedRestoration(ctx, original, binding, cause)
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

func TestRestorationConclusionUsesItsOwnersAndPreservesUncertainty(t *testing.T) {
	activation := testsupport.ServerActivation("original", "contributor", plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("release")})
	binding, err := plugins.DecodeActivation(activation)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		attempted    frame.BindingSnapshot
		cause        error
		outer        error
		cancel       bool
		hostCanceled bool
		definite     bool
	}{
		{name: "admission read", cause: errors.New("activation read failed")},
		{name: "preparation read", attempted: binding, cause: errors.New("activation storage read failed")},
		{name: "caller cancellation", attempted: binding, cause: context.Canceled, cancel: true},
		{name: "caller cancellation with closed connection", attempted: binding, cause: sdk.ErrConnectionClosed, cancel: true},
		{name: "recovery barrier", attempted: binding, cause: frame.ErrRecoveryRequired},
		{name: "runner uncertainty", attempted: binding, cause: runner.ErrUnknown},
		{name: "unavailable and unknown", attempted: binding, cause: errors.Join(frame.ErrUnavailable, runner.ErrUnknown)},
		{name: "stale and recovery required", attempted: binding, cause: errors.Join(frame.ErrStaleBinding, frame.ErrRecoveryRequired)},
		{name: "SDK cleanup cancellation", attempted: binding, cause: errors.Join(sdk.ErrConnectionClosed, context.Canceled), definite: true},
		{name: "SDK cleanup deadline", attempted: binding, cause: errors.Join(sdk.ErrConnectionClosed, context.DeadlineExceeded), definite: true},
		{name: "outer cancellation marker", attempted: binding, cause: sdk.ErrConnectionClosed, outer: context.Canceled, definite: true},
		{name: "Host cancellation", attempted: binding, cause: sdk.ErrConnectionClosed, hostCanceled: true},
		{name: "outer recovery required", attempted: binding, cause: sdk.ErrConnectionClosed, outer: frame.ErrRecoveryRequired},
		{name: "closed connection and unconfirmed release", attempted: binding, cause: errors.Join(sdk.ErrConnectionClosed, frame.ErrUnconfirmedRelease)},
	} {
		t.Run(test.name, func(t *testing.T) {
			installed := &Installed{owner: frame.NewLifetime(t.Context())}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.cancel {
				cancel()
			}
			if test.hostCanceled {
				installed.owner.BeginClose(frame.ErrClosed)
				<-installed.owner.Context().Done()
			}
			name := testsupport.InstallationMCPServer(activation.Dependencies[0].InstallationID, "contributor")
			connections := &connections{lifetime: t.Context()}
			pool := &Pool{inner: connections}
			attempt, err := connections.beginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*launchPermit, error) {
				return nil, errors.Join(installed.failedRestoration(ctx, activation, test.attempted, test.cause), test.outer)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := attempt.Run(); !errors.Is(err, test.cause) {
				t.Fatalf("attempt lost its original cause: %v", err)
			}
			definite, err := pool.RestorationFailure(name, activation)
			if definite != test.definite || !errors.Is(err, test.cause) || (test.outer != nil && !errors.Is(err, test.outer)) {
				t.Fatalf("restoration conclusion = %t, %v; want definite=%t", definite, err, test.definite)
			}
			if err := connections.shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
