package bootstrap

import (
	"fmt"

	skillspec "github.com/Tangerg/scope/skills"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/maintenance"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func buildRunMaintenance(
	cfg Config,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
	memoryCuration *agentmemory.Curation,
	resolveUtility modeladapter.AuxiliaryResolver,
	contextState maintenance.SessionContextInvalidator,
) (executionadapter.RunMaintenance, executionadapter.ModelContextCompactor, error) {
	compactor, err := maintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		maintenance.NewLiveStateSnapshotter(shells),
		maintenance.CompactionPolicyValues{},
		contextState,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build compactor: %w", err)
	}
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor, nil
	}
	consolidator, err := maintenance.NewMemoryConsolidator(
		conversationServices.store,
		memoryCuration,
		resolveUtility,
		maintenance.MemoryCurationPolicyValues{},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build memory consolidator: %w", err)
	}
	skillRepository, repositoryErr := skillspec.NewDirectoryRepository(
		cfg.SkillsUserDir,
		skillspec.RepositoryConfig{},
	)
	if repositoryErr != nil {
		return nil, nil, fmt.Errorf("runtime: open user skill repository: %w", repositoryErr)
	}
	skillMiner, err := maintenance.NewSkillProposalMiner(
		conversationServices.store,
		skills,
		skillRepository,
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
	pipeline, err := maintenance.NewPipeline(consolidator, skillMiner, skillArchiver)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build maintenance pipeline: %w", err)
	}
	return pipeline, compactor, nil
}
