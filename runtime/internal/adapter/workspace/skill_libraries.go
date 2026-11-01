package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

// SkillLibraries confines publication to the admitted user or project root.
type SkillLibraries struct {
	user     *skillauthoring.Store
	projects sync.Map
	journal  string
}

func NewSkillLibraries(user *skillauthoring.Store, journal string) (*SkillLibraries, error) {
	if user == nil || !filepath.IsAbs(journal) {
		return nil, errors.New("workspace: user skill library is required")
	}
	return &SkillLibraries{user: user, journal: journal}, nil
}

func (l *SkillLibraries) Library(_ context.Context, root string, scope skills.Scope) (string, error) {
	store, err := l.store(scope, root)
	if err != nil {
		return "", err
	}
	return store.Library(), nil
}
func (l *SkillLibraries) Preimage(ctx context.Context, root string, proposal skills.Proposal) (string, error) {
	store, err := l.store(proposal.Scope, root)
	if err != nil {
		return "", err
	}
	return store.Preimage(ctx, proposal)
}
func (l *SkillLibraries) Publish(ctx context.Context, root string, proposal skills.Proposal, previous, permit string) ([]string, error) {
	store, err := l.store(proposal.Scope, root)
	if err != nil {
		return nil, err
	}
	return store.Publish(ctx, proposal, previous, permit, l.journal)
}

func (l *SkillLibraries) store(scope skills.Scope, projectRoot string) (*skillauthoring.Store, error) {
	switch scope {
	case skills.ScopeUser:
		return l.user, nil
	case skills.ScopeProject:
		if strings.TrimSpace(projectRoot) == "" {
			return nil, errors.New("workspace Skill libraries: project root is required")
		}
		if loaded, ok := l.projects.Load(projectRoot); ok {
			store, ok := loaded.(*skillauthoring.Store)
			if !ok {
				return nil, errors.New("workspace Skill libraries: project cache contains an invalid entry")
			}
			return store, nil
		}
		created, err := skillauthoring.NewProjectStore(projectRoot, promptsource.ProjectSkillDir(projectRoot))
		if err != nil {
			return nil, err
		}
		loaded, _ := l.projects.LoadOrStore(projectRoot, created)
		store, ok := loaded.(*skillauthoring.Store)
		if !ok {
			return nil, errors.New("workspace Skill libraries: project cache contains an invalid entry")
		}
		return store, nil
	default:
		return nil, errors.New("workspace Skill libraries: invalid Skill scope")
	}
}
