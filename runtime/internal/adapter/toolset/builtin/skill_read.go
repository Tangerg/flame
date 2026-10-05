// Skill readers provide model-facing Skill discovery and resource access.
package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"

	skillspec "github.com/Tangerg/scope/skills"
	skillstool "github.com/Tangerg/scope/tools/skills"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// SkillUsageRecorder records that a skill was loaded, feeding the idle-lifecycle
// curator's last-used signal. nil disables use recording.
type SkillUsageRecorder interface {
	RecordUse(ctx context.Context, name string, now time.Time) error
}

// BuildReaders assembles the working-directory-scoped reading tools over the
// merged skill source (project <cwd>/.flame/skills layered over the user dir,
// project winning). It returns nil when neither directory exists, so a session
// that ships no skills gets no skill tools at all. When recorder is non-nil,
// loading a skill records a use so the curator can tell active skills from idle
// ones.
//
// Rebuilt per resolution because the project directory depends on the Run's
// working directory.
func BuildReaders(ctx context.Context, cwd, userDir string, recorder SkillUsageRecorder, packages promptsource.PackageSkills) ([]toolcontract.Tool, []plugin.Dependency, error) {
	var decorateUser func(skillspec.ResourceSource) skillspec.ResourceSource
	if recorder != nil {
		// Wrap only the user source: the curator governs the user library, and
		// merge resolves a shadowed name to the project copy, so this records
		// exactly the user-resolved loads (a project skill never touches the
		// user-library usage record).
		decorateUser = func(user skillspec.ResourceSource) skillspec.ResourceSource {
			return recordingSource{ResourceSource: user, recorder: recorder}
		}
	}
	source, dependencies, err := promptsource.OverlaySkillSource(ctx, cwd, userDir, packages, decorateUser)
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

// recordingSource records successful user-library Skill loads. A usage-write
// failure emits a diagnostic without changing the loaded Skill.
type recordingSource struct {
	skillspec.ResourceSource
	recorder SkillUsageRecorder
}

func (r recordingSource) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	skill, err := r.ResourceSource.Load(ctx, name)
	if err == nil {
		if usageErr := r.recorder.RecordUse(ctx, name, time.Now()); usageErr != nil {
			slog.ErrorContext(ctx, "skill: record usage failed", "skill.name", name, "error", usageErr)
		}
	}
	return skill, err
}
