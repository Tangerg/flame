package maintenance

import (
	"context"
	"errors"

	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/samber/lo"
)

// MemoryMaintenance runs after the successful Run commits. The selected Memory
// backend owns its extraction and curation decisions.
type MemoryMaintenance interface {
	MaintainMemory(ctx context.Context, input executionadapter.RunMaintenanceInput, compacted bool) error
}

type SkillsMaintenance interface {
	MaintainSkills(ctx context.Context, input executionadapter.RunMaintenanceInput, completedToolCalls int64) error
}

type runWorkReader interface {
	CompletedRunWork(ctx context.Context, sessionID, rootRunID string) (completedToolCalls int64, compacted bool, err error)
}

type Pipeline struct {
	work   runWorkReader
	memory MemoryMaintenance
	skills SkillsMaintenance
}

func NewPipeline(work runWorkReader, memory MemoryMaintenance, skills SkillsMaintenance) (*Pipeline, error) {
	if lo.IsNil(work) || lo.IsNil(memory) || lo.IsNil(skills) {
		return nil, errors.New("maintenance: committed Run work and both providers are required")
	}
	return &Pipeline{work: work, memory: memory, skills: skills}, nil
}

// Maintain completes one best-effort maintenance pass. Memory consolidation is
// cost-amortized behind a durable summary already produced by the exact
// model-request compaction path; this pipeline never makes a second context
// reduction decision from a partial request projection.
func (p *Pipeline) Maintain(ctx context.Context, input executionadapter.RunMaintenanceInput) executionadapter.RunMaintenanceResult {
	completedTools, compacted, err := p.work.CompletedRunWork(ctx, input.SessionID, input.RootRunID)
	if err != nil {
		return executionadapter.RunMaintenanceResult{Errors: []error{err}}
	}
	result := executionadapter.RunMaintenanceResult{}
	if err := p.skills.MaintainSkills(ctx, input, completedTools); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := p.memory.MaintainMemory(ctx, input, compacted); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
