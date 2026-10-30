package run

import (
	"fmt"
	"strings"
)

// LossReason names why a Run's real outcome cannot be established. Clients
// word it; Detail beside it is only a diagnostic.
type LossReason string

const (
	// LossRestart is a Run that was not parked at a waiting boundary when its
	// Runtime stopped.
	LossRestart LossReason = "restart"
	// LossIsolatedWorkspace is a Run whose isolated scratch workspace does not
	// survive the executor that created it.
	LossIsolatedWorkspace LossReason = "isolatedWorkspace"
	// LossWorkspaceUnavailable is a Run whose workspace cannot be restored.
	LossWorkspaceUnavailable LossReason = "workspaceUnavailable"
	// LossOtherBuild is waiting state written by a different Runtime build.
	LossOtherBuild LossReason = "otherBuild"
	// LossWaitingStateUnavailable is waiting state that is missing, malformed
	// or not at a waiting boundary.
	LossWaitingStateUnavailable LossReason = "waitingStateUnavailable"
	// LossConfigurationChanged is a Run whose Session or execution
	// configuration no longer matches the state it waited in.
	LossConfigurationChanged LossReason = "configurationChanged"
	// LossUnresolvedEffects is a Run whose external operations have no
	// provable durable result.
	LossUnresolvedEffects LossReason = "unresolvedEffects"
	// LossUnresolvedModelCall is a Run whose model invocation ended without a
	// provable durable result.
	LossUnresolvedModelCall LossReason = "unresolvedModelCall"
)

func (l LossReason) Valid() bool {
	switch l {
	case LossRestart, LossIsolatedWorkspace, LossWorkspaceUnavailable, LossOtherBuild,
		LossWaitingStateUnavailable, LossConfigurationChanged, LossUnresolvedEffects, LossUnresolvedModelCall:
		return true
	default:
		return false
	}
}

// Loss explains a lost Run.
type Loss struct {
	Reason LossReason
	Detail string
}

func (l Loss) Validate() error {
	if !l.Reason.Valid() {
		return fmt.Errorf("run: unknown loss reason %q", l.Reason)
	}
	if l.Detail != strings.TrimSpace(l.Detail) {
		return fmt.Errorf("run: loss detail has surrounding whitespace")
	}
	return nil
}
