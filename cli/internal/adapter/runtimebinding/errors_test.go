package runtimebinding

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type runtimeProblemError struct {
	data  protocol.ProblemData
	cause error
}

func requireRuntimeContractViolation(t testing.TB, err error) {
	t.Helper()
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("error = %v, want ErrIncompatibleRuntime", err)
	}
}

func (r runtimeProblemError) Error() string                 { return r.data.Type }
func (r runtimeProblemError) Unwrap() error                 { return r.cause }
func (r runtimeProblemError) Problem() protocol.ProblemData { return r.data }

func TestRuntimeContractViolationPreservesValidationCause(t *testing.T) {
	cause := &protocol.ConstraintError{
		Shape:  "RuntimeEvent",
		Fields: []protocol.FieldError{{Field: "sequence", Detail: "must be positive"}},
	}
	err := runtimeContractViolation("runtime change event is invalid: %v", cause)
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("error = %v, want ErrIncompatibleRuntime", err)
	}
	var preserved *protocol.ConstraintError
	if !errors.As(err, &preserved) || preserved != cause {
		t.Fatalf("validation cause = %v, want original ConstraintError", preserved)
	}
	want := agent.ErrIncompatibleRuntime.Error() + ": runtime change event is invalid: " + cause.Error()
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestClassifyErrorPreservesIdentityAndProjectsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	source := runtimeProblemError{
		cause: protocol.ErrCapabilityNotNeg,
		data: protocol.ProblemData{
			Type: protocol.ErrCapabilityNotNeg.Error(), Detail: "declare the missing topic",
			DocURL: "https://docs.example/discovery",
			RequiredCapabilities: []protocol.CapabilityRequirement{{
				Type: protocol.RequirementRuntimeTopic, Name: "files.changed",
			}},
		},
	}
	err := classifyError(source)
	if !errors.Is(err, agent.ErrIncompatibleRuntime) || !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("classified identities = %v", err)
	}
	var wire protocol.ProblemError
	if !errors.As(err, &wire) || wire.Problem().Type != protocol.ErrCapabilityNotNeg.Error() {
		t.Fatalf("runtime problem was not preserved: %T %v", err, err)
	}
	for _, want := range []string{"declare the missing topic", "docs.example", "runtimeTopic:files.changed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("projected error omitted %q: %s", want, err)
		}
	}
}

func TestClassifyErrorExposesCommandReplaySemantics(t *testing.T) {
	for _, test := range []struct {
		source error
		want   error
	}{
		{source: protocol.ErrIdempotencyInProgress, want: agent.ErrCommandInProgress},
		{source: protocol.ErrIdempotencyConflict, want: agent.ErrCommandConflict},
		{source: protocol.ErrIdempotencyStoreMismatch, want: agent.ErrCommandStoreMismatch},
	} {
		problem := protocol.ProblemData{Type: test.source.Error()}
		if errors.Is(test.source, protocol.ErrIdempotencyInProgress) {
			problem.RetryAfterSeconds = 1
		}
		err := classifyError(runtimeProblemError{cause: test.source, data: problem})
		if !errors.Is(err, test.want) || !errors.Is(err, test.source) {
			t.Fatalf("classified %v = %v, want %v", test.source, err, test.want)
		}
	}
}

func TestClassifyErrorProjectsUnmappedRuntimeProblem(t *testing.T) {
	t.Parallel()

	source := runtimeProblemError{
		cause: errors.New("provider cause"),
		data: protocol.ProblemData{
			Type: protocol.ProblemRateLimited, RetryAfterSeconds: 4,
		},
	}
	err := classifyError(source)
	if !errors.Is(err, source.cause) {
		t.Fatalf("unmapped runtime problem lost its cause: %v", err)
	}
	for _, want := range []string{protocol.ProblemRateLimited, "retry after 4s"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("unmapped runtime problem omitted %q: %v", want, err)
		}
	}
}

func TestClassifyErrorRejectsMalformedRuntimeProblem(t *testing.T) {
	t.Parallel()
	source := runtimeProblemError{
		cause: errors.New("malformed provider cause"),
		data:  protocol.ProblemData{Type: protocol.ProblemRateLimited, RetryAfterSeconds: -1},
	}
	err := classifyError(source)
	requireRuntimeContractViolation(t, err)
	if !errors.Is(err, source.cause) {
		t.Fatalf("contract violation lost the runtime cause: %v", err)
	}
}
