package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame/cancellation"
)

type failedDefinitionRegistry struct {
	sourceRegistry
	err error
}

func (r failedDefinitionRegistry) Definition(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return mcpserver.Server{}, false, r.err
}

func TestInstallationActivationPreservesDefinitionFailures(t *testing.T) {
	origin, err := mcpserver.InstallationOrigin(testsupport.InstallationID(t))
	if err != nil {
		t.Fatal(err)
	}
	id, err := mcpserver.NewID(origin, testsupport.ServerName("workflow"))
	if err != nil {
		t.Fatal(err)
	}
	storageFailure := errors.New("installation read failed")
	canceled, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("activation request withdrawn")
	cancel(cause)
	cancellations := cancellation.Err(canceled)
	for _, operation := range []struct {
		name string
		call func(*Pool, context.Context, mcpserver.ID) (plugin.Activation, error)
	}{
		{name: "current", call: (*Pool).InstallationActivation},
		{name: "await", call: (*Pool).AwaitInstallationActivation},
	} {
		for _, failure := range []error{storageFailure, cancellations, errors.Join(storageFailure, cancellations)} {
			t.Run(operation.name+"/"+failure.Error(), func(t *testing.T) {
				pool := &Pool{registry: failedDefinitionRegistry{err: failure}}
				_, err := operation.call(pool, t.Context(), id)
				if err != failure || errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, ErrUnavailable) {
					t.Fatalf("definition failure = %v, want original %v without source-loss classification", err, failure)
				}
			})
		}
		t.Run(operation.name+"/missing", func(t *testing.T) {
			pool := &Pool{registry: failedDefinitionRegistry{}}
			if _, err := operation.call(pool, t.Context(), id); !errors.Is(err, plugin.ErrUnavailable) || !errors.Is(err, ErrUnavailable) {
				t.Fatalf("missing source = %v, want unavailable", err)
			}
		})
	}
}

func TestWaitSessionPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("wait withdrawn")
	cancel(cause)
	owner := &connections{lifetime: t.Context()}
	session, err := owner.waitSession(ctx, mcpserver.Server{})
	if session != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("canceled activation wait = %v, %v, want only caller cancellation", session, err)
	}
}
