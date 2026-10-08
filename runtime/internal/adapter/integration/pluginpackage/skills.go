package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	sdk "github.com/Tangerg/scope/skills"
)

type skillInstallations interface {
	List(context.Context) ([]plugin.Snapshot, error)
	Get(context.Context, resourceid.InstallationID) (plugin.Snapshot, error)
}
type Skills struct {
	releases      *Releases
	installations skillInstallations
}

func NewSkills(releases *Releases, installations skillInstallations) *Skills {
	return &Skills{releases: releases, installations: installations}
}
func (s *Skills) SkillBundles(ctx context.Context) ([]promptsource.PackageSkillBundle, error) {
	installations, err := s.installations.List(ctx)
	if err != nil {
		return nil, err
	}
	var bundles []promptsource.PackageSkillBundle
	for _, snapshot := range installations {
		installation, release := snapshot.Installation, snapshot.Selected
		if !installation.Active() {
			continue
		}
		names, err := installation.EnabledSkills(release)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			continue
		}
		root, err := s.releases.Root(release.Digest())
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, promptsource.PackageSkillBundle{Root: filepath.Join(root, "skills"), Names: names, Dependency: installation.Dependency()})
	}
	return bundles, nil
}

func (s *Skills) ReadSkillResource(ctx context.Context, dependency plugin.Dependency, name, resource string) ([]byte, error) {
	if err := sdk.ValidateName(name); err != nil {
		return nil, fmt.Errorf("%w: Skill name: %w", plugin.ErrInvalid, err)
	}
	if !fs.ValidPath(resource) || strings.ContainsRune(resource, '\\') {
		return nil, fmt.Errorf("%w: Skill resource identity", plugin.ErrInvalid)
	}
	snapshot, err := s.installations.Get(ctx, dependency.InstallationID)
	if err != nil {
		if errors.Is(err, plugin.ErrNotFound) {
			return nil, errors.Join(workspaceapp.ErrSkillUnavailable, err)
		}
		return nil, err
	}
	installation, release := snapshot.Installation, snapshot.Selected
	if installation.Dependency() != dependency {
		return nil, errors.Join(workspaceapp.ErrSkillUnavailable, plugin.ErrStale)
	}
	enabled, err := installation.EnabledSkills(release)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(enabled, name) {
		return nil, workspaceapp.ErrSkillUnavailable
	}
	limit := int64(domainskills.MaxSkillResourceBytes)
	if resource == sdk.SkillFile {
		limit = domainskills.MaxAuthoredSkillDocumentBytes
	}
	content, err := s.releases.readResource(ctx, installation, "skills/"+name+"/"+resource, limit)
	if context.Cause(ctx) != nil {
		return nil, context.Cause(ctx)
	}
	if errors.Is(err, errResourceLimit) {
		if resource == sdk.SkillFile {
			return nil, domainskills.ErrDocumentTooLarge
		}
		return nil, domainskills.ErrResourceTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", workspaceapp.ErrSkillUnavailable, err)
	}
	return content, nil
}
