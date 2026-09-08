package maintenance

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
)

// Pipeline composes the post-Run maintenance workers. It keeps the lifecycle
// policy beside the concrete workers: mine the transcript for Skill proposals,
// archive idle Skills, then consolidate memory only after the model-call path
// actually summarized durable context.
type Pipeline struct {
	consolidator  *MemoryConsolidator
	skillMiner    *SkillProposalMiner
	skillArchiver *IdleSkillArchiver
}

// NewPipeline composes the default maintenance workers for clean Run endings.
func NewPipeline(consolidator *MemoryConsolidator, skillMiner *SkillProposalMiner, skillArchiver *IdleSkillArchiver) (*Pipeline, error) {
	if consolidator == nil || skillMiner == nil || skillArchiver == nil {
		return nil, errors.New("maintenance: all pipeline workers are required")
	}
	return &Pipeline{
		consolidator:  consolidator,
		skillMiner:    skillMiner,
		skillArchiver: skillArchiver,
	}, nil
}

// Maintain completes one best-effort maintenance pass. Memory consolidation is
// cost-amortized behind a durable summary already produced by the exact
// model-request compaction path; this pipeline never makes a second context
// reduction decision from a partial request projection.
func (p *Pipeline) Maintain(ctx context.Context, input agentexec.RunMaintenanceInput) agentexec.RunMaintenanceResult {
	result := agentexec.RunMaintenanceResult{}
	if err := p.skillMiner.MineIfDue(ctx, input.SessionID, input.CWD, input.ToolCalls); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := p.skillArchiver.ArchiveIfDue(ctx); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if !input.DurableContextCompacted {
		return result
	}
	if err := p.consolidator.Consolidate(ctx, input.SessionID, input.CWD); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
