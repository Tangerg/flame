package bootstrap

import (
	"fmt"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/maintenance"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func buildRunMaintenance(
	cfg assemblyConfig,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills maintenance.SkillsMaintenance,
	memory maintenance.MemoryMaintenance,
	resolveUtility modeladapter.AuxiliaryResolver,
) (executionadapter.RunMaintenance, executionadapter.ModelContextCompactor, error) {
	compactor, err := maintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		maintenance.NewLiveStateSnapshotter(shells),
		maintenance.CompactionPolicyValues{},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build compactor: %w", err)
	}
	if cfg.maintenance != nil {
		return cfg.maintenance, compactor, nil
	}
	pipeline, err := maintenance.NewPipeline(memory, skills)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build maintenance pipeline: %w", err)
	}
	return pipeline, compactor, nil
}
