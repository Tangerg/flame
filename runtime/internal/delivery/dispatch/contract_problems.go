package dispatch

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type ProblemChannel string

const (
	problemChannelRPC       ProblemChannel = "rpc"
	ProblemChannelExecution ProblemChannel = "execution"
)

type ProblemContract struct {
	Type              string
	Channels          []ProblemChannel
	Required          []string
	Optional          []string
	ExecutionRecovery protocol.RecoveryAction
}

var executionRecovery = map[string]protocol.RecoveryAction{protocol.ProblemInternalError: protocol.RecoveryPromptUser, protocol.ProblemAgentStuck: protocol.RecoveryPromptUser, protocol.ProblemRateLimited: protocol.RecoveryWaitRetryAfter, protocol.ProblemInvalidCredentials: protocol.RecoveryReauthenticate, protocol.ProblemTimeout: protocol.RecoveryPromptUser, protocol.ProblemProviderUnavailable: protocol.RecoveryWaitRetryAfter, protocol.ProblemProviderRejected: protocol.RecoveryStop, protocol.ProblemDeniedByUser: protocol.RecoveryStop, protocol.ProblemToolFailed: protocol.RecoveryPromptUser, protocol.ProblemToolCanceled: protocol.RecoveryStop, protocol.ProblemChildRunCanceled: protocol.RecoveryStop}

var problemContracts = mustProblemContracts()

func ProblemContracts() []ProblemContract {
	out := slices.Clone(problemContracts)
	for index := range out {
		out[index].Channels = slices.Clone(out[index].Channels)
		out[index].Required = slices.Clone(out[index].Required)
		out[index].Optional = slices.Clone(out[index].Optional)
	}
	return out
}

func mustProblemContracts() []ProblemContract {
	byType := make(map[string]*ProblemContract)
	add := func(channel ProblemChannel, types ...string) {
		for _, problemType := range types {
			contract := byType[problemType]
			if contract == nil {
				contract = &ProblemContract{Type: problemType}
				byType[problemType] = contract
			}
			if !slices.Contains(contract.Channels, channel) {
				contract.Channels = append(contract.Channels, channel)
			}
		}
	}

	add(problemChannelRPC, protocol.ProblemTypes()...)
	add(ProblemChannelExecution, slices.Sorted(maps.Keys(executionRecovery))...)

	common := []string{"detail", "docUrl"}
	out := make([]ProblemContract, 0, len(byType))
	for _, contract := range byType {
		contract.Optional = slices.Clone(common)
		contract.ExecutionRecovery = executionRecovery[contract.Type]
		switch contract.Type {
		case protocol.ErrInvalidParams.Error():
			contract.Optional = append(contract.Optional, "errors")
		case protocol.ErrCapabilityNotNeg.Error():
			contract.Required = []string{"requiredCapabilities"}
		case protocol.ErrSessionHasActiveRun.Error():
			contract.Required = []string{"activeRun"}
		case protocol.ErrIdempotencyInProgress.Error():
			contract.Required = []string{"retryAfterSeconds"}
		case protocol.ProblemRateLimited, protocol.ProblemTimeout, protocol.ProblemProviderUnavailable:
			contract.Optional = append(contract.Optional, "retryAfterSeconds")
		}
		out = append(out, *contract)
	}
	slices.SortFunc(out, func(left, right ProblemContract) int {
		return cmp.Compare(left.Type, right.Type)
	})
	for index, contract := range out {
		if err := contract.validate(); err != nil {
			panic(fmt.Sprintf("dispatch: invalid problem contract %d: %v", index, err))
		}
	}
	return out
}

func (p ProblemContract) validate() error {
	if p.Type == "" {
		return errors.New("problem type is empty")
	}
	if len(p.Channels) == 0 {
		return fmt.Errorf("problem type %q has no channel", p.Type)
	}

	for index, channel := range p.Channels {
		if slices.Contains(p.Channels[:index], channel) {
			return fmt.Errorf("problem type %q repeats channel %q", p.Type, channel)
		}
		switch channel {
		case problemChannelRPC, ProblemChannelExecution:
		default:
			return fmt.Errorf("problem type %q has unknown channel %q", p.Type, channel)
		}
	}
	return nil
}
