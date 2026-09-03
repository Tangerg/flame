package runtimebinding

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type scriptedRuntimeLifecycle struct {
	results []error
	closes  int
}

func (s *scriptedRuntimeLifecycle) Close() error {
	result := s.results[s.closes]
	s.closes++
	return result
}

func TestOwnerRetainsFailedConnectionUntilCleanupCompletes(t *testing.T) {
	openFailure := errors.New("negotiation failed")
	closeFailure := errors.New("cleanup incomplete")
	lifecycle := &scriptedRuntimeLifecycle{results: []error{closeFailure, nil}}
	failed := &Connection{lifecycle: lifecycle}
	owner := NewOwner(Config{})

	err := owner.rejectOpen(failed, openFailure)
	if !errors.Is(err, openFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("reject open error = %v", err)
	}
	if owner.connection != failed || !owner.closing {
		t.Fatal("owner did not retain the incompletely closed Runtime")
	}
	if _, err := owner.Connection(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("Connection during cleanup = %v, want ErrDisconnected", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("resume cleanup: %v", err)
	}
	if lifecycle.closes != 2 || owner.connection != nil {
		t.Fatalf("cleanup result = (%d closes, connection %p)", lifecycle.closes, owner.connection)
	}
}

func TestOwnerDoesNotRetainFailedConnectionAfterSuccessfulCleanup(t *testing.T) {
	openFailure := errors.New("negotiation failed")
	lifecycle := &scriptedRuntimeLifecycle{results: []error{nil}}
	owner := NewOwner(Config{})

	err := owner.rejectOpen(&Connection{lifecycle: lifecycle}, openFailure)
	if !errors.Is(err, openFailure) {
		t.Fatalf("reject open error = %v", err)
	}
	if owner.connection != nil || owner.closing {
		t.Fatal("successful cleanup changed owner state")
	}
}
