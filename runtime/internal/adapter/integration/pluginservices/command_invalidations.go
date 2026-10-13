package pluginservices

import (
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
)

// The feature consumers translate captured contracts into their existing read
// invalidations. An outcome notice never supplies a competing business result.
func CommandInvalidations(publish invalidation.Publish) (func(plugin.CommandBinding), error) {
	memory, err := memoryrpc.NewContracts()
	if err != nil {
		return nil, err
	}
	schedules, err := schedulerpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return func(record plugin.CommandBinding) {
		switch record.Contract {
		case memory.Capability.Identity():
			publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
		case schedules.Capability.Identity():
			publish.Notify(invalidation.ForSchedules())
		}
	}, nil
}
