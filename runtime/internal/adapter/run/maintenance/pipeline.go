package maintenance

import (
	"context"
	"errors"

	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
)

// MemoryMaintenance runs after successful Scope Interaction completion, before
// committing the Flame Segment terminal fact. The selected Memory backend owns
// its extraction and curation decisions.
type MemoryMaintenance interface {
	MaintainMemory(context.Context, executionadapter.RunMaintenanceInput) error
}

type Pipeline struct {
	consolidator  MemoryMaintenance
	skillMiner    *SkillProposalMiner
	skillArchiver *IdleSkillArchiver
}

// NewPipeline composes maintenance workers for successful Scope Interaction completion.
func NewPipeline(consolidator MemoryMaintenance, skillMiner *SkillProposalMiner, skillArchiver *IdleSkillArchiver) (*Pipeline, error) {
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
func (p *Pipeline) Maintain(ctx context.Context, input executionadapter.RunMaintenanceInput) executionadapter.RunMaintenanceResult {
	result := executionadapter.RunMaintenanceResult{}
	if err := p.skillMiner.MineIfDue(ctx, input.SessionID, input.WorkspaceCWD, input.ToolCalls); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := p.skillArchiver.ArchiveIfDue(ctx); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := p.consolidator.MaintainMemory(ctx, input); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
