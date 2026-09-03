package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

var (
	// ErrSkillProposalsUnavailable reports that proposal review is unavailable.
	ErrSkillProposalsUnavailable = errors.New("workspace: skill proposals unavailable")
	// ErrSkillLibraryUnavailable reports that Skill-library curation is unavailable.
	ErrSkillLibraryUnavailable = errors.New("workspace: skill library unavailable")
)

// SkillCatalog enumerates skills visible from a working directory.
type SkillCatalog interface {
	List(ctx context.Context, cwd string) ([]SkillSummary, error)
}

// SkillCurator manages active and archived user-authored Skills. Mutation
// methods return the exact opaque file identities whose public projection
// changed, including changes committed before a later error.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) ([]string, error)
	Restore(ctx context.Context, name string) ([]string, error)
}

// SkillProposals stores the one current immutable proposal for each scoped
// Skill name. Mutation methods return exact opaque file identities so
// filesystem observation can accept only the caller's committed paths without
// swallowing concurrent external edits.
type SkillProposals interface {
	SubmitProposal(ctx context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, []string, error)
	ListProposals(ctx context.Context, projectRoot string) ([]skills.ProposalReview, error)
	ApproveProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error)
	RejectProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error)
}

// Skills owns discovery, library curation, and proposal review.
type Skills struct {
	scope         *Scope
	catalog       SkillCatalog
	curator       SkillCurator
	proposals     SkillProposals
	observations  *AuthoredWatch
	invalidations invalidation.Publish
}

// NewSkills builds interactive Skill discovery, curation, and review use cases.
func NewSkills(scope *Scope, catalog SkillCatalog, curator SkillCurator, proposals SkillProposals, observations *AuthoredWatch, invalidations invalidation.Publish) *Skills {
	return &Skills{
		scope: scope, catalog: catalog, curator: curator, proposals: proposals,
		observations: observations, invalidations: invalidations,
	}
}

// List enumerates the one precedence-resolved Skill per name visible from cwd,
// ordered by name.
func (s *Skills) List(ctx context.Context, cwd string) ([]SkillSummary, error) {
	root, err := s.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if s.catalog == nil {
		return nil, nil
	}
	found, err := s.catalog.List(ctx, root)
	if err != nil {
		return nil, err
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(first, second SkillSummary) int {
		return cmp.Compare(first.Name, second.Name)
	})
	for index := 1; index < len(found); index++ {
		if found[index].Name == found[index-1].Name {
			return nil, fmt.Errorf("workspace: discovered Skill catalog repeats visible name %q", found[index].Name)
		}
	}
	return found, nil
}

// Managed returns active and archived user-authored Skills.
func (s *Skills) Managed(ctx context.Context) ([]skills.Entry, error) {
	if s.curator == nil {
		return nil, ErrSkillLibraryUnavailable
	}
	return s.curator.List(ctx)
}

// Archive removes a Skill from active use without deleting it.
func (s *Skills) Archive(ctx context.Context, name string) error {
	if s.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	identities, err := s.curator.Archive(ctx, name)
	s.publishSkillMutation(identities)
	return err
}

// Restore returns an archived Skill to active use.
func (s *Skills) Restore(ctx context.Context, name string) error {
	if s.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	identities, err := s.curator.Restore(ctx, name)
	s.publishSkillMutation(identities)
	return err
}

// SubmitProposal submits immutable Skill content without activating it.
func (s *Skills) SubmitProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	if s.proposals == nil {
		return skills.ProposalRef{}, ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	ref, identities, err := s.proposals.SubmitProposal(ctx, root, proposal)
	s.publishSkillMutation(identities)
	return ref, err
}

// Proposals returns the current immutable Skill proposals visible from cwd,
// ordered by scope and name.
func (s *Skills) Proposals(ctx context.Context, cwd string) ([]skills.ProposalReview, error) {
	if s.proposals == nil {
		return nil, ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	proposals, err := s.proposals.ListProposals(ctx, root)
	if err != nil {
		return nil, err
	}
	proposals = slices.Clone(proposals)
	slices.SortFunc(proposals, func(first, second skills.ProposalReview) int {
		return cmp.Or(
			cmp.Compare(string(first.Ref.Scope), string(second.Ref.Scope)),
			cmp.Compare(first.Ref.Name, second.Ref.Name),
		)
	})
	for index := 1; index < len(proposals); index++ {
		previous, current := proposals[index-1].Ref, proposals[index].Ref
		if current.Scope == previous.Scope && current.Name == previous.Name {
			return nil, fmt.Errorf("workspace: skill proposal catalog repeats current slot %s/%s", current.Scope, current.Name)
		}
	}
	return proposals, nil
}

// ApproveProposal accepts a Skill proposal into its target library.
func (s *Skills) ApproveProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if s.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return err
	}
	identities, err := s.proposals.ApproveProposal(ctx, root, ref)
	s.publishSkillMutation(identities)
	return err
}

// RejectProposal removes a Skill proposal without activating it.
func (s *Skills) RejectProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if s.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return err
	}
	identities, err := s.proposals.RejectProposal(ctx, root, ref)
	s.publishSkillMutation(identities)
	return err
}

func (s *Skills) publishSkillMutation(identities []string) {
	if len(identities) == 0 {
		return
	}
	if s.observations != nil {
		s.observations.Accept(AuthoredChange{Resource: AuthoredSkills, Identities: identities})
	}
	s.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
}
