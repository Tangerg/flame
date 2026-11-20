package pluginservices

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Counters follow the Runtime outcome; advice alone cannot advance them.
const skillMetricsScope = "scope/flame/skill"

var minedSkillProposalCounter = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(skillMetricsScope).Int64Counter("skill.proposals.mined",
		metric.WithDescription("Skill proposals submitted by the maintenance pipeline, by kind (new/revise)."))
	return counter
})

var archivedIdleSkillCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(skillMetricsScope).Int64Counter("skill.idle.archived",
		metric.WithDescription("Idle agent-authored Skills archived by the maintenance pipeline."))
	return counter
})

// minedSkillKind is the metric label telling a new Skill from a revision.
type minedSkillKind string

const (
	minedNewSkill  minedSkillKind = "new"
	minedRevision  minedSkillKind = "revise"
	skillKindLabel                = "skill.kind"
)

func recordMinedSkillProposal(ctx context.Context, revises bool) {
	kind := minedNewSkill
	if revises {
		kind = minedRevision
	}
	minedSkillProposalCounter().Add(ctx, 1, metric.WithAttributes(attribute.String(skillKindLabel, string(kind))))
}

func recordArchivedIdleSkills(ctx context.Context, count int) {
	if count > 0 {
		archivedIdleSkillCounter().Add(ctx, int64(count))
	}
}
