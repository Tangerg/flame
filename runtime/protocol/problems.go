package protocol

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/optional"
)

type problemSpec struct {
	sentinel          error
	retryAfterSeconds int
	methodDeclarable  bool
	recovery          RecoveryAction
}

var problemSpecs = mustProblemSpecs([]problemSpec{
	frameworkProblem(ErrInvalidRequest, RecoveryStop),
	frameworkProblem(ErrInternalError, RecoveryStop),
	frameworkProblem(ErrMethodNotFound, RecoveryStop),
	frameworkProblem(ErrInvalidParams, RecoveryStop),
	declaredProblem(ErrSessionNotFound, RecoveryRefetch),
	declaredProblem(ErrRunNotFound, RecoveryRefetch),
	declaredProblem(ErrItemNotFound, RecoveryRefetch),
	declaredProblem(ErrPluginUnavailable, RecoveryPromptUser),
	declaredProblem(ErrPluginNotFound, RecoveryPromptUser),
	declaredProblem(ErrPluginInUse, RecoveryPromptUser),
	declaredProblem(ErrPluginDisabled, RecoveryPromptUser),
	declaredProblem(ErrPluginStale, RecoveryPromptUser),
	declaredProblem(ErrPluginCommandRetired, RecoveryStop),
	declaredProblem(ErrPluginChanged, RecoveryPromptUser),
	declaredProblem(ErrMCPOwnedByInstallation, RecoveryPromptUser),
	declaredProblem(ErrMCPServerNotFound, RecoveryRefetch),
	declaredProblem(ErrSkillNotFound, RecoveryRefetch),
	declaredProblem(ErrSkillUnavailable, RecoveryPromptUser),
	declaredProblem(ErrMCPServerAlreadyExists, RecoveryRefetch),
	declaredProblem(ErrMCPServerDisabled, RecoveryRefetch),
	declaredProblem(ErrMCPAuthorizationAttemptNotFound, RecoveryStop),
	declaredProblem(ErrRunNotRoot, RecoveryRefetch),
	declaredProblem(ErrSessionBusy, RecoveryRefetch),
	declaredProblem(ErrRevisionConflict, RecoveryRefetch),
	declaredProblem(ErrInterruptNotOpen, RecoveryColdRecover),
	declaredProblem(ErrReplayUnavailable, RecoveryColdRecover),
	declaredProblem(ErrRunWaiting, RecoveryColdRecover),
	declaredProblem(ErrRunFinished, RecoveryColdRecover),
	declaredProblem(ErrStaleSegment, RecoveryRefetch),
	declaredProblem(ErrReplayCursorInvalid, RecoveryResubscribe),
	declaredProblem(ErrSessionHasActiveRun, RecoveryPromptUser),
	declaredProblem(ErrCapabilityNotNeg, RecoveryPromptUser),
	declaredProblem(ErrWorkspaceUnavailable, RecoveryPromptUser),
	declaredProblem(ErrCheckpointUnavailable, RecoveryPromptUser),
	declaredProblem(ErrCheckpointConflict, RecoveryPromptUser),
	declaredProblem(ErrPromptSourceTooLarge, RecoveryPromptUser),
	declaredProblem(ErrExportTooLarge, RecoveryPromptUser),
	declaredProblem(ErrUnsupportedMime, RecoveryPromptUser),
	declaredProblem(ErrPathOutsideRoot, RecoveryPromptUser),
	declaredProblem(ErrVcsUnavailable, RecoveryPromptUser),
	frameworkProblem(ErrInvalidProtocolVersion, RecoveryPromptUser),
	frameworkProblem(ErrIdempotencyConflict, RecoveryPromptUser),
	frameworkProblem(ErrIdempotencyStoreMismatch, RecoveryColdRecover),
	declaredProblem(ErrProviderError, RecoveryPromptUser),
	retryingProblem(ErrIdempotencyInProgress, 1),
})

func frameworkProblem(sentinel error, recovery RecoveryAction) problemSpec {
	return problemSpec{sentinel: sentinel, recovery: recovery}
}

func declaredProblem(sentinel error, recovery RecoveryAction) problemSpec {
	return problemSpec{sentinel: sentinel, methodDeclarable: true, recovery: recovery}
}

func retryingProblem(sentinel error, retryAfterSeconds int) problemSpec {
	return problemSpec{
		sentinel:          sentinel,
		retryAfterSeconds: retryAfterSeconds,
		recovery:          RecoveryWaitRetryAfter,
	}
}

// Failure is a safely projected operation failure. Bindings expose Problem while
// errors.Is still reaches stable protocol and context sentinels through Unwrap.
type Failure struct {
	cause error
	data  ProblemData
}

var _ ProblemError = (*Failure)(nil)

type ProblemDetailed interface {
	error
	Enrich(*ProblemData)
}

func (f *Failure) Error() string {
	if f.data.Detail != "" {
		return f.data.Detail
	}
	return f.data.Type
}

func (f *Failure) Unwrap() error { return f.cause }

// Problem returns a defensive copy of the client-visible problem.
func (f *Failure) Problem() ProblemData {
	return cloneProblemData(f.data)
}

// NewFailure separates a client-safe explanation from the cause chain, which
// must identify a registered problem. Structured details remain owner-provided.
func NewFailure(cause error, detail string) *Failure {
	spec, ok := problemSpecForError(cause)
	if !ok {
		return internalFailure()
	}
	failure := newFailure(spec, cause, detail)
	// Internal output defects retain their diagnostic cause without projecting
	// that cause's input fields onto the internal-error wire shape.
	if detailed, ok := errors.AsType[ProblemDetailed](cause); ok && spec.sentinel != ErrInternalError {
		detailed.Enrich(&failure.data)
	}
	if spec.sentinel == ErrInvalidParams {
		if constraint, ok := errors.AsType[*ConstraintError](cause); ok {
			failure.data.Errors = slices.Clone(constraint.Fields)
		}
	}
	if ValidateWireTree(failure.data) != nil {
		return internalFailure()
	}
	return failure
}

// ProjectError maps an implementation error onto the binding-neutral problem
// vocabulary without exposing arbitrary internal details for unknown failures.
func ProjectError(err error) *Failure {
	if err == nil {
		return nil
	}
	if failure, ok := errors.AsType[*Failure](err); ok {
		if ValidateWireTree(failure.data) != nil {
			return internalFailure()
		}
		return &Failure{cause: failure.cause, data: cloneProblemData(failure.data)}
	}
	for _, spec := range problemSpecs {
		if !errors.Is(err, spec.sentinel) {
			continue
		}
		detail := ""
		if err != spec.sentinel {
			detail = err.Error()
		}
		failure := NewFailure(err, detail)
		if ValidateWireTree(failure.data) != nil {
			return internalFailure()
		}
		return failure
	}
	failure := internalFailure()
	// Keep cancellation actionable for in-process callers without retaining
	// arbitrary endpoint details or adding request lifetime to the wire problem.
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			failure.cause = errors.Join(failure.cause, cause)
		}
	}
	return failure
}

// InvalidParameters projects strict wire-constraint failures with their field
// locations intact.
func InvalidParameters(err error) *Failure {
	return ProjectError(NewFailure(errors.Join(ErrInvalidParams, err), err.Error()))
}

func newFailure(spec problemSpec, cause error, detail string) *Failure {
	return &Failure{
		cause: cause,
		data: ProblemData{
			Type:              spec.sentinel.Error(),
			Detail:            detail,
			RetryAfterSeconds: spec.retryAfterSeconds,
		},
	}
}

func internalFailure() *Failure {
	spec, _ := problemSpecForType(ProblemInternalError)
	return newFailure(spec, ErrInternalError, "the runtime could not complete the request")
}

// DecodeFailure restores a wire problem through the authoritative sentinel
// registry. Invalid remote data is a decoding failure, not an operation failure.
func DecodeFailure(data ProblemData) (*Failure, error) {
	spec, ok := problemSpecForType(data.Type)
	if !ok {
		return nil, errors.New("unknown operation problem type")
	}
	if err := ValidateWireTree(data); err != nil {
		return nil, err
	}
	return &Failure{cause: spec.sentinel, data: cloneProblemData(data)}, nil
}

func cloneProblemData(data ProblemData) ProblemData {
	data.RequiredCapabilities = slices.Clone(data.RequiredCapabilities)
	data.Errors = slices.Clone(data.Errors)
	data.ActiveRun = optional.Clone(data.ActiveRun)
	return data
}

// RecoveryFor returns the safe default recovery action for a problem type.
func RecoveryFor(problemType string) (RecoveryAction, bool) {
	spec, ok := problemSpecForType(problemType)
	return spec.recovery, ok
}

// IsMethodProblemType reports whether method metadata may declare the type.
func IsMethodProblemType(problemType string) bool {
	spec, ok := problemSpecForType(problemType)
	return ok && spec.methodDeclarable
}

// ProblemTypes returns every registered first-party problem type.
func ProblemTypes() []string {
	out := make([]string, 0, len(problemSpecs))
	for _, spec := range problemSpecs {
		out = append(out, spec.sentinel.Error())
	}
	return out
}

func RetryAfterFor(problemType string) int {
	spec, _ := problemSpecForType(problemType)
	return spec.retryAfterSeconds
}

func problemSpecForError(err error) (problemSpec, bool) {
	for _, spec := range problemSpecs {
		if errors.Is(err, spec.sentinel) {
			return spec, true
		}
	}
	return problemSpec{}, false
}

func problemSpecForType(problemType string) (problemSpec, bool) {
	for _, spec := range problemSpecs {
		if spec.sentinel.Error() == problemType {
			return spec, true
		}
	}
	return problemSpec{}, false
}

func mustProblemSpecs(specs []problemSpec) []problemSpec {
	seen := make(map[string]bool, len(specs))
	for index, spec := range specs {
		switch {
		case spec.sentinel == nil:
			panic(fmt.Sprintf("operation: problem spec %d has no sentinel", index))
		case seen[spec.sentinel.Error()]:
			panic(fmt.Sprintf("operation: problem type %q is registered twice", spec.sentinel))
		case !spec.recovery.Valid():
			panic(fmt.Sprintf("operation: problem type %q has invalid recovery %q", spec.sentinel, spec.recovery))
		case spec.recovery == RecoveryWaitRetryAfter && spec.retryAfterSeconds <= 0:
			panic(fmt.Sprintf("operation: problem type %q waits without retryAfterSeconds", spec.sentinel))
		case spec.recovery != RecoveryWaitRetryAfter && spec.retryAfterSeconds != 0:
			panic(fmt.Sprintf("operation: problem type %q publishes retryAfterSeconds for recovery %q", spec.sentinel, spec.recovery))
		}
		seen[spec.sentinel.Error()] = true
	}
	return specs
}
