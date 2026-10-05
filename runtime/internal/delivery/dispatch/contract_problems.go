package dispatch

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ProblemChannel names where a first-party ProblemData type may ride. RPC
// problems have a numeric JSON-RPC code; execution problems ride run/item/tool
// outcomes. MCP connection status is not a ProblemData carrier: its closed
// vocabulary is protocol.MCPStatusProblemType.
type ProblemChannel string

const (
	ProblemChannelRPC       ProblemChannel = "rpc"
	ProblemChannelExecution ProblemChannel = "execution"
)

// ProblemContract is the single first-party error catalog. Required and Optional
// describe the complete ProblemData frame for Type; Channels describe its legal
// carriers. The union generator and manifest consume the same rows, so adding a
// problem cannot update validation while silently omitting discovery metadata.
type ProblemContract struct {
	Type     string
	Channels []ProblemChannel
	Required []string
	Optional []string
}

var problemContracts = mustProblemContracts()

// ProblemContracts returns a snapshot of the first-party error catalog.
func ProblemContracts() []ProblemContract {
	out := slices.Clone(problemContracts)
	for index := range out {
		out[index].Channels = slices.Clone(out[index].Channels)
		out[index].Required = slices.Clone(out[index].Required)
		out[index].Optional = slices.Clone(out[index].Optional)
	}
	return out
}

// ProblemTypesFor returns the stable symbolic vocabulary of one carrier.
func ProblemTypesFor(channel ProblemChannel) []string {
	switch channel {
	case ProblemChannelRPC, ProblemChannelExecution:
	default:
		panic(fmt.Sprintf("dispatch: unknown problem channel %q", channel))
	}
	var out []string
	for _, contract := range problemContracts {
		if slices.Contains(contract.Channels, channel) {
			out = append(out, contract.Type)
		}
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

	add(ProblemChannelRPC, delivery.ProblemTypes()...)
	add(ProblemChannelExecution,
		protocol.ProblemInternalError,
		protocol.ProblemRunLost,
		protocol.ProblemAgentStuck,
		protocol.ProblemRateLimited,
		protocol.ProblemInvalidAPIKey,
		protocol.ProblemTimeout,
		protocol.ProblemProviderUnavailable,
		protocol.ProblemProviderRejected,
		protocol.ProblemDeniedByUser,
		protocol.ProblemToolFailed,
		protocol.ProblemToolCanceled,
		protocol.ProblemChildRunCanceled,
	)

	common := []string{"detail", "docUrl"}
	out := make([]ProblemContract, 0, len(byType))
	for _, contract := range byType {
		contract.Optional = slices.Clone(common)
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
		case ProblemChannelRPC, ProblemChannelExecution:
		default:
			return fmt.Errorf("problem type %q has unknown channel %q", p.Type, channel)
		}
	}
	return nil
}
