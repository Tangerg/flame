package promptsource

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/skill"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	app "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// A captured command pins the provider's choices across Interaction recovery.
// The host retains actual readers and verifies every returned name and source.
type SkillCapture struct {
	Command    string
	Activation plugin.Activation
	Restore    bool
}
type DiscoverSkills func(context.Context, skilllearning.Candidates, *SkillCapture) (skilllearning.Catalog, error)
type SkillSources struct {
	Packages PackageSkills
	Discover DiscoverSkills
	Capture  *SkillCapture
}

type candidates struct {
	readers   map[skilllearning.Choice]selectedSkill
	input     skilllearning.Candidates
	installed map[string]bool
}

func (c *candidates) add(name, source string, scope skill.Scope, reader selectedSkill) error {
	ref := skilllearning.Choice{Name: name, Source: source}
	if _, exists := c.readers[ref]; exists {
		return errors.New("skill discovery: duplicate admitted source")
	}
	c.readers[ref] = reader
	c.input.Items = append(c.input.Items, skilllearning.Candidate{Name: name, Source: source, Scope: scope})
	if scope == skill.Installation {
		c.installed[name] = true
	}
	return nil
}
func (c *candidates) directories(ctx context.Context, user, project *SkillSource) error {
	for _, layer := range []struct {
		source *SkillSource
		origin app.SkillSource
		scope  skill.Scope
	}{
		{user, app.UserSkillSource(), skill.User}, {project, app.ProjectSkillSource(), skill.Project},
	} {
		if layer.source == nil {
			continue
		}
		entries, err := layer.source.directoryEntries(ctx)
		if err != nil {
			return err
		}
		for _, name := range skillCandidateNames(entries) {
			if err := c.add(name, string(layer.scope), layer.scope, selectedSkill{source: layer.source, origin: layer.origin}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *candidates) bundles(bundles []PackageSkillBundle, packages PackageSkills) error {
	for _, bundle := range bundles {
		origin, err := app.InstallationSkillSource(bundle.Dependency)
		if err != nil {
			return err
		}
		source := &packageSkillSource{bundle: bundle, authority: packages}
		id := bundle.Dependency.InstallationID.String() + "/" + bundle.Dependency.Digest.String()
		for _, bundled := range bundle.Skills {
			if err := c.add(bundled.Name, id, skill.Installation, selectedSkill{source: source, origin: origin}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *candidates) selectReaders(ctx context.Context, sources SkillSources) (*resolvedSkills, error) {
	if sources.Discover == nil {
		return nil, nil
	}
	slices.SortFunc(c.input.Items, func(a, b skilllearning.Candidate) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Source, b.Source))
	})
	result, err := sources.Discover(ctx, c.input, sources.Capture)
	if err != nil {
		return nil, err
	}
	selected := &resolvedSkills{byName: make(map[string]selectedSkill)}
	for _, choice := range result.Entries {
		if _, duplicate := selected.byName[choice.Name]; duplicate {
			return nil, app.ErrSkillUnavailable
		}
		reader, found := c.readers[choice]
		restored := sources.Capture != nil && sources.Capture.Restore
		if !found && (choice.Source != "" || (!c.installed[choice.Name] && !restored)) {
			return nil, app.ErrSkillUnavailable
		}
		reader.packageCopy = c.installed[choice.Name]
		selected.byName[choice.Name] = reader
	}
	return selected, nil
}
