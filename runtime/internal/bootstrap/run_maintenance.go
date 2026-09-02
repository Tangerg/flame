package bootstrap

import (
	"fmt"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/maintenance"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func buildRunMaintenance(
	cfg Config,
	defaultSelection modelref.Selection,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
	memoryCuration *agentmemory.Curation,
	resolveUtility modeladapter.AuxiliaryResolver,
	contextState maintenance.SessionContextInvalidator,
) (agentexec.RunMaintenance, agentexec.ModelContextCompactor, error) {
	fallbackLimits := modelref.TokenLimits{}
	limits, found, err := modeladapter.LookupTokenLimits(defaultSelection)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: default model token limits: %w", err)
	}
	if found {
		fallbackLimits = limits
	}
	compactor, err := maintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		maintenance.NewLiveStateSnapshotter(shells),
		maintenance.CompactionPolicyValues{FallbackTokenLimits: fallbackLimits},
		contextState,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: build compactor: %w", err)
	}
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor, nil
	}
	var consolidator *maintenance.MemoryConsolidator
	if memoryCuration.Available() {
		consolidator, err = maintenance.NewMemoryConsolidator(
			conversationServices.store,
			memoryCuration,
			resolveUtility,
			maintenance.MemoryCurationPolicyValues{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: build memory consolidator: %w", err)
		}
	}
	var skillMiner *maintenance.SkillProposalMiner
	var skillArchiver *maintenance.IdleSkillArchiver
	if skillMaintenance.Available() {
		skillRepository, repositoryErr := skillspec.NewDirectoryRepository(
			cfg.SkillsUserDir,
			skillspec.RepositoryConfig{},
		)
		if repositoryErr != nil {
			return nil, nil, fmt.Errorf("runtime: open user skill repository: %w", repositoryErr)
		}
		skillMiner, err = maintenance.NewSkillProposalMiner(
			conversationServices.store,
			skills,
			skillRepository,
			resolveUtility,
			maintenance.SkillMiningPolicyValues{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: build skill proposal miner: %w", err)
		}
		skillArchiver, err = maintenance.NewIdleSkillArchiver(skillMaintenance, maintenance.SkillArchivePolicyValues{})
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: build idle skill archiver: %w", err)
		}
	}
	return maintenance.NewPipeline(compactor, consolidator, skillMiner, skillArchiver), compactor, nil
}
