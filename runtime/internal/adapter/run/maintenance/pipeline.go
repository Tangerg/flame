package maintenance

import (
	"context"
	"errors"

	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/samber/lo"
)

// MemoryMaintenance runs after successful Scope Interaction completion, before
// committing the Flame Segment terminal fact. The selected Memory backend owns
// its extraction and curation decisions.
type MemoryMaintenance interface {
	MaintainMemory(context.Context, executionadapter.RunMaintenanceInput) error
}

type SkillsMaintenance interface {
	MaintainSkills(context.Context, executionadapter.RunMaintenanceInput) error
}

type Pipeline struct {
	memory MemoryMaintenance
	skills SkillsMaintenance
}

func NewPipeline(memory MemoryMaintenance, skills SkillsMaintenance) (*Pipeline, error) {
	if lo.IsNil(memory) || lo.IsNil(skills) {
		return nil, errors.New("maintenance: all pipeline workers are required")
	}
	return &Pipeline{memory: memory, skills: skills}, nil
}

// Maintain completes one best-effort maintenance pass. Memory consolidation is
// cost-amortized behind a durable summary already produced by the exact
// model-request compaction path; this pipeline never makes a second context
// reduction decision from a partial request projection.
func (p *Pipeline) Maintain(ctx context.Context, input executionadapter.RunMaintenanceInput) executionadapter.RunMaintenanceResult {
	result := executionadapter.RunMaintenanceResult{}
	if err := p.skills.MaintainSkills(ctx, input); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := p.memory.MaintainMemory(ctx, input); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
