package builtin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	toolcontract "github.com/Tangerg/scope/core/tool"
	skillspec "github.com/Tangerg/scope/skills"
	skillstool "github.com/Tangerg/scope/tools/skills"
	"github.com/samber/lo"
)

type SkillUsageRecorder interface {
	RecordUse(ctx context.Context, name string, now time.Time) error
}

func BuildReaders(ctx context.Context, cwd, userDir string, recorder SkillUsageRecorder, packages promptsource.SkillSources, retained []plugin.Dependency) ([]toolcontract.Tool, []plugin.Dependency, error) {
	if lo.IsNil(recorder) {
		return nil, nil, errors.New("skill: usage recorder is required")
	}
	decorateUser := func(user skillspec.ResourceSource) skillspec.ResourceSource {
		return recordingSource{ResourceSource: user, recorder: recorder}
	}
	source, dependencies, err := promptsource.OverlaySkillSource(ctx, cwd, userDir, packages, decorateUser, retained)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: build source: %w", err)
	}
	if source == nil {
		return nil, nil, nil
	}
	tools, err := skillstool.NewTools(source, skillstool.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("skill: build tools: %w", err)
	}
	return tools, dependencies, nil
}

type recordingSource struct {
	skillspec.ResourceSource
	recorder SkillUsageRecorder
}

func (r recordingSource) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	skill, err := r.ResourceSource.Load(ctx, name)
	if err == nil {
		if usageErr := r.recorder.RecordUse(ctx, name, time.Now()); usageErr != nil {
			slog.ErrorContext(ctx, "skill: record usage failed", slog.Any("skill.name", name), telemetry.Error(usageErr))
		}
	}
	return skill, err
}
