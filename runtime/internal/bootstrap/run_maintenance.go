package bootstrap

import (
	"fmt"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/maintenance"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func buildRunMaintenance(
	cfg Config,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
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
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor, nil
	}
	skillSource, err := promptsource.NewSkillSource(cfg.SkillsUserDir, cfg.SkillsUserDir)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build user skill source: %w", err)
	}
	skillMiner, err := maintenance.NewSkillProposalMiner(
		conversationServices.store,
		skills,
		skillSource,
		resolveUtility,
		maintenance.SkillMiningPolicyValues{},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build skill proposal miner: %w", err)
	}
	skillArchiver, err := maintenance.NewIdleSkillArchiver(skillMaintenance, maintenance.SkillArchivePolicyValues{})
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build idle skill archiver: %w", err)
	}
	pipeline, err := maintenance.NewPipeline(memory, skillMiner, skillArchiver)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build maintenance pipeline: %w", err)
	}
	return pipeline, compactor, nil
}
