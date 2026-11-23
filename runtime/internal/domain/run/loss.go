package run

import (
	"fmt"
	"strings"
)

type LossReason string

const (
	LossRestart                 LossReason = "restart"
	LossIsolatedWorkspace       LossReason = "isolatedWorkspace"
	LossWorkspaceUnavailable    LossReason = "workspaceUnavailable"
	LossOtherBuild              LossReason = "otherBuild"
	LossWaitingStateUnavailable LossReason = "waitingStateUnavailable"
	LossConfigurationChanged    LossReason = "configurationChanged"
	LossUnresolvedEffects       LossReason = "unresolvedEffects"
	LossUnresolvedModelCall     LossReason = "unresolvedModelCall"
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

func (l LossReason) DeclaredAfterStop() bool {
	switch l {
	case LossUnresolvedEffects, LossUnresolvedModelCall:
		return false
	default:
		return true
	}
}

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
