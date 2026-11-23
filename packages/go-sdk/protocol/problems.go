package protocol

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

type Failure struct {
	cause error
	data  ProblemData
}

type problemDetailed interface {
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

func (f *Failure) Problem() ProblemData {
	return cloneProblemData(f.data)
}

func NewFailure(kind error, detail string, causes ...error) *Failure {
	if kind == nil {
		return internalFailure()
	}
	spec, ok := problemSpecForType(kind.Error())
	if !ok {
		return internalFailure()
	}
	cause := errors.Join(append([]error{kind}, causes...)...)
	failure := newFailure(spec, cause, detail)
	if detailed, ok := errors.AsType[problemDetailed](cause); ok && !errors.Is(spec.sentinel, ErrInternalError) && errors.Is(detailed, kind) {
		detailed.Enrich(&failure.data)
	}
	if errors.Is(spec.sentinel, ErrInvalidParams) {
		if constraint, ok := errors.AsType[*ConstraintError](cause); ok {
			failure.data.Errors = slices.Clone(constraint.Fields)
		}
	}
	if ValidateWireTree(failure.data) != nil {
		return internalFailure()
	}
	return failure
}

func ProjectError(err error) *Failure {
	if err == nil {
		return nil
	}
	projected := internalFailure()
	if failure, ok := errors.AsType[*Failure](err); ok {
		if ValidateWireTree(failure.data) == nil {
			projected = &Failure{cause: failure.cause, data: cloneProblemData(failure.data)}
		}
	} else {
		for _, spec := range problemSpecs {
			if !errors.Is(err, spec.sentinel) {
				continue
			}
			detail := ""
			if message := err.Error(); message != spec.sentinel.Error() {
				detail = message
			}
			projected = NewFailure(spec.sentinel, detail, err)
			break
		}
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) && !errors.Is(projected, cause) {
			projected.cause = errors.Join(projected.cause, cause)
		}
	}
	return projected
}

func InvalidParameters(err error) *Failure {
	return ProjectError(NewFailure(ErrInvalidParams, err.Error(), err))
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
	if data.ActiveRun != nil {
		data.ActiveRun = new(*data.ActiveRun)
	}
	return data
}

func RecoveryFor(problemType string) (RecoveryAction, bool) {
	spec, ok := problemSpecForType(problemType)
	return spec.recovery, ok
}

func IsMethodProblemType(problemType string) bool {
	spec, ok := problemSpecForType(problemType)
	return ok && spec.methodDeclarable
}

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
