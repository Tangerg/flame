package pluginpackage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"

	skillspec "github.com/Tangerg/scope/skills"
)

func parseSkills(ctx context.Context, root *os.Root, release *plugin.Builder) error {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	dir, err := fs.ReadDir(root.FS(), skillsDirectory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		report(release, plugin.Component{Kind: plugin.ComponentSkills}, plugin.DiagnosticUnavailableComponent)
		if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
			return errors.Join(err, cause)
		}
		return nil
	}
	for _, entry := range dir {
		if !entry.IsDir() {
			continue
		}
		content, err := read(ctx, root, path.Join(skillsDirectory, entry.Name(), skillspec.SkillFile), domainskills.MaxAuthoredSkillDocumentBytes)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
				return errors.Join(err, cause)
			}
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, plugin.DiagnosticUnavailableComponent)
			continue
		}
		skill, err := skilldocument.Load(ctx, entry.Name(), content)
		if err != nil {
			if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
				return errors.Join(err, cause)
			}
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, plugin.DiagnosticInvalidDeclaration)
			continue
		}
		code, admitted := admission(release.AdmitSkill(plugin.Skill{Name: skill.Name, Description: skill.Description}))
		switch {
		case admitted:
		case code == plugin.DiagnosticComponentLimit:
			report(release, plugin.Component{Kind: plugin.ComponentSkills}, code)
			return nil
		default:
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, code)
		}
	}
	return nil
}
