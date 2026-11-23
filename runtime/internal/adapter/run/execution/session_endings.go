package execution

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) endingForProcess(owner scoperun.ProcessID, termination scoperun.Termination) run.Ending {
	if termination.Status() == scoperun.StatusFailed ||
		termination.Cause() == scoperun.TerminationCauseHostCancellation && termination.Reason() == modelProcessStopReason {
		if classified, found := i.modelFailures.lookup(owner); found {
			return run.Failed{Failure: classified}
		}
	}
	return terminationEnding(termination)
}

func terminationEnding(termination scoperun.Termination) run.Ending {
	failed := func(kind run.FailureKind, detail string) run.Ending {
		return run.Failed{Failure: run.Failure{Kind: kind, Detail: detail}}
	}
	switch termination.Cause() {
	case scoperun.TerminationCauseCompletion:
		return run.Completed{}
	case scoperun.TerminationCauseParentDeadline, scoperun.TerminationCauseHostDeadline:
		return run.TimedOut{Detail: "executor deadline reached"}
	case scoperun.TerminationCauseParentCancellation, scoperun.TerminationCauseHostCancellation:
		if termination.Cause() == scoperun.TerminationCauseHostCancellation && termination.Reason() == unresolvedEffectsStopReason {
			return run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedEffects}}
		}
		return run.Canceled{}
	case scoperun.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		return failed(run.FailureAgentStuck, run.Diagnostic(errors.New(failure.Message())))
	case scoperun.TerminationCauseExternalFailure:
		failure, _ := termination.Failure()
		detail := run.Diagnostic(errors.New(failure.Message()))
		switch failure.Classification() {
		case interaction.ErrInvalidModelResponse.Classification(), interaction.FailureToolCallsNotCompleted:
			return failed(run.FailureProviderRejected, detail)
		}
		return failed(run.FailureInternal, detail)
	case scoperun.TerminationCauseContractFailure, scoperun.TerminationCausePanic:
		failure, _ := termination.Failure()
		return failed(run.FailureInternal, run.Diagnostic(errors.New(failure.Message())))
	case scoperun.TerminationCauseRuntimeKill:
		return failed(run.FailureInternal, termination.Reason())
	default:
		return failed(run.FailureInternal, "executor returned an unknown terminal cause")
	}
}

const unresolvedEffectsStopReason = "runtime unresolved external effects"
