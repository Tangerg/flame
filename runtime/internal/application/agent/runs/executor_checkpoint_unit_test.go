package runs

import (
	"errors"
	"testing"
)

func TestExecutorCheckpointValidatesOnlyApplicationEnvelope(t *testing.T) {
	valid := ExecutorCheckpoint{
		RootMemberID: "root",
		SessionID:    "session-1",
		Payload:      []byte(`{"executorOwned":"opaque"}`),
		BuildID:      testExecutorBuildID,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExecutorCheckpoint)
	}{
		{name: "empty root", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = "" }},
		{name: "unstable root", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = " root" }},
		{name: "empty payload", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Payload = nil }},
		{name: "empty build", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.BuildID = "" }},
		{name: "empty session", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.SessionID = "" }},
		{name: "unstable session", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.SessionID = " session-1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkpoint := valid.Clone()
			test.mutate(&checkpoint)
			if err := checkpoint.Validate(); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("Validate error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}

func TestExecutorCheckpointCloneOwnsMutableData(t *testing.T) {
	original := ExecutorCheckpoint{
		RootMemberID: "root",
		SessionID:    "session-1",
		Payload:      []byte("payload"),
		BuildID:      testExecutorBuildID,
	}
	clone := original.Clone()
	clone.Payload[0] = 'P'
	if string(original.Payload) != "payload" {
		t.Fatalf("Clone shares mutable storage with original: %+v", original)
	}
}

func TestExecutorCheckpointValidatesCrossAggregateOwnership(t *testing.T) {
	checkpoint := ExecutorCheckpoint{
		RootMemberID: "member-root",
		SessionID:    "session-1",
		Payload:      []byte("opaque"),
		BuildID:      testExecutorBuildID,
	}
	if err := checkpoint.ValidateOwnership("member-root", "session-1"); err != nil {
		t.Fatalf("ValidateOwnership: %v", err)
	}
	for _, mismatch := range []struct{ name, root, session string }{
		{name: "root", root: "other-root", session: "session-1"},
		{name: "session", root: "member-root", session: "other-session"},
	} {
		t.Run(mismatch.name, func(t *testing.T) {
			if err := checkpoint.ValidateOwnership(mismatch.root, mismatch.session); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("ValidateOwnership error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}
