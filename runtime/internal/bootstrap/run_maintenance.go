package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chatclient"
	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/flame/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/flame/runtime/internal/adapter/runmaintenance"
	"github.com/Tangerg/flame/runtime/internal/application/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/exec"
)

func buildRunMaintenance(
	cfg Config,
	defaultSelection modelref.Selection,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
	memoryCuration *agentmemory.Curation,
	resolveUtility func(context.Context) *chatclient.Client,
) (agentexec.RunMaintenance, agentexec.ModelContextCompactor, error) {
	fallbackLimits := modelref.TokenLimits{}
	limits, found, err := modelcatalog.LookupTokenLimits(defaultSelection)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: default model token limits: %w", err)
	}
	if found {
		fallbackLimits = limits
	}
	compactor, err := runmaintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		runmaintenance.NewLiveStateSnapshotter(shells),
		runmaintenance.CompactionPolicyValues{FallbackTokenLimits: fallbackLimits},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: compaction policy: %w", err)
	}
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor, nil
	}
	var consolidator *runmaintenance.MemoryConsolidator
	if memoryCuration.Available() {
		consolidator, err = runmaintenance.NewMemoryConsolidator(
			conversationServices.store,
			memoryCuration,
			resolveUtility,
			runmaintenance.MemoryCurationPolicyValues{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: memory curation policy: %w", err)
		}
	}
	var skillMiner *runmaintenance.SkillProposalMiner
	var skillArchiver *runmaintenance.IdleSkillArchiver
	if skillMaintenance.Available() {
		skillMiner, err = runmaintenance.NewSkillProposalMiner(
			conversationServices.store,
			skills,
			skillspec.NewDirectoryRepository(cfg.SkillsUserDir),
			resolveUtility,
			runmaintenance.SkillMiningPolicyValues{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: skill mining policy: %w", err)
		}
		skillArchiver, err = runmaintenance.NewIdleSkillArchiver(skillMaintenance, runmaintenance.SkillArchivePolicyValues{})
		if err != nil {
			return nil, nil, fmt.Errorf("runtime: skill archive policy: %w", err)
		}
	}
	return runmaintenance.NewPipeline(compactor, consolidator, skillMiner, skillArchiver), compactor, nil
}
