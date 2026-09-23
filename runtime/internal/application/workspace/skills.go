package workspace

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

var ErrSkillUnavailable = errors.New("workspace: skill is unavailable")

// SkillCatalog enumerates skills visible from a working directory. List
// transfers ownership of the returned summaries to the caller.
type SkillCatalog interface {
	List(ctx context.Context, cwd string) (SkillDiscovery, error)
	Get(ctx context.Context, cwd, name string) (SkillDetail, error)
}

// SkillCurator manages the one active or archived entry for each user-authored
// Skill name. Mutation methods return the exact opaque file identities whose
// public projection changed, including changes committed before a later error.
// List transfers ownership of the returned entries to the caller.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) ([]string, error)
	Restore(ctx context.Context, name string) ([]string, error)
}

// SkillProposals stores the one current immutable proposal for each scoped
// Skill name. Mutation methods return exact opaque file identities so
// filesystem observation can accept only the caller's committed paths without
// swallowing concurrent external edits. ListProposals transfers ownership of
// the returned reviews to the caller.
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
func NewSkills(scope *Scope, catalog SkillCatalog, curator SkillCurator, proposals SkillProposals, observations *AuthoredWatch, invalidations invalidation.Publish) (*Skills, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "catalog", value: catalog},
		{name: "curator", value: curator},
		{name: "proposal store", value: proposals},
		{name: "authored observation", value: observations},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("workspace: skills %s is required", required.name)
		}
	}
	return &Skills{
		scope: scope, catalog: catalog, curator: curator, proposals: proposals,
		observations: observations, invalidations: invalidations,
	}, nil
}

// List enumerates the one precedence-resolved Skill per name visible from cwd,
// ordered by name.
func (s *Skills) List(ctx context.Context, cwd string) (SkillDiscovery, error) {
	root, err := s.scope.root(cwd)
	if err != nil {
		return SkillDiscovery{}, err
	}
	catalog, err := s.catalog.List(ctx, root)
	if err != nil {
		return SkillDiscovery{}, err
	}
	found := catalog.Skills
	if len(found) > 2*skills.MaxSkillsPerSource {
		return SkillDiscovery{}, fmt.Errorf("%w: discovered catalog contains %d Skills", skills.ErrLibraryCapacity, len(found))
	}
	for index, entry := range found {
		if err := validateSkillSummary(entry); err != nil {
			return SkillDiscovery{}, fmt.Errorf("workspace: discovered Skill %d is invalid: %w", index+1, err)
		}
	}
	slices.SortFunc(found, func(first, second SkillSummary) int {
		return cmp.Compare(first.Name, second.Name)
	})
	for index := 1; index < len(found); index++ {
		if found[index].Name == found[index-1].Name {
			return SkillDiscovery{}, fmt.Errorf("workspace: discovered Skill catalog repeats visible name %q", found[index].Name)
		}
	}
	if len(catalog.Diagnostics) > 2*skills.MaxSkillDirectoryEntries {
		return SkillDiscovery{}, fmt.Errorf("%w: too many discovery diagnostics", skills.ErrLibraryCapacity)
	}
	seen := make(map[string]bool, len(found))
	for _, entry := range found {
		seen[entry.Name] = true
	}
	for _, diagnostic := range catalog.Diagnostics {
		if strings.TrimSpace(diagnostic.Name) == "" || strings.TrimSpace(diagnostic.Detail) == "" || len(diagnostic.Detail) > 512 || !utf8.ValidString(diagnostic.Detail) || seen[diagnostic.Name] {
			return SkillDiscovery{}, fmt.Errorf("workspace: invalid or repeated Skill diagnostic %q", diagnostic.Name)
		}
		seen[diagnostic.Name] = true
	}
	slices.SortFunc(catalog.Diagnostics, func(a, b SkillDiagnostic) int { return cmp.Compare(a.Name, b.Name) })
	return catalog, nil
}

func (s *Skills) Get(ctx context.Context, cwd, name string) (SkillDetail, error) {
	if err := skills.ValidateName(name); err != nil {
		return SkillDetail{}, err
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return SkillDetail{}, err
	}
	detail, err := s.catalog.Get(ctx, root, name)
	if err != nil {
		return SkillDetail{}, err
	}
	if err := validateSkillSummary(detail.SkillSummary); err != nil {
		return SkillDetail{}, err
	}
	revision, err := hex.DecodeString(detail.Revision)
	if err != nil || len(revision) != 32 || detail.Name != name || strings.TrimSpace(detail.Path) == "" || !utf8.ValidString(detail.Instructions) || len(detail.Instructions) > skills.MaxAuthoredSkillDocumentBytes {
		return SkillDetail{}, fmt.Errorf("workspace: invalid Skill detail %q", name)
	}
	return detail, nil
}

// Managed returns active and archived user-authored Skills, active first and
// then ordered by name within each lifecycle.
func (s *Skills) Managed(ctx context.Context) ([]skills.Entry, error) {
	entries, err := s.curator.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(entries) > skills.MaxSkillsPerSource {
		return nil, fmt.Errorf("%w: managed catalog contains %d Skills", skills.ErrLibraryCapacity, len(entries))
	}
	for index, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("workspace: managed Skill %d is invalid: %w", index+1, err)
		}
	}
	slices.SortFunc(entries, func(first, second skills.Entry) int {
		return cmp.Or(
			cmp.Compare(string(first.Lifecycle), string(second.Lifecycle)),
			cmp.Compare(first.Name, second.Name),
		)
	})
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, duplicate := seen[entry.Name]; duplicate {
			return nil, fmt.Errorf("workspace: managed Skill catalog repeats name %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
	}
	return entries, nil
}

// Archive removes a Skill from active use without deleting it.
func (s *Skills) Archive(ctx context.Context, name string) error {
	if err := skills.ValidateName(name); err != nil {
		return err
	}
	identities, err := s.curator.Archive(ctx, name)
	s.publishSkillMutation(identities)
	return err
}

// Restore returns an archived Skill to active use.
func (s *Skills) Restore(ctx context.Context, name string) error {
	if err := skills.ValidateName(name); err != nil {
		return err
	}
	identities, err := s.curator.Restore(ctx, name)
	s.publishSkillMutation(identities)
	return err
}

// SubmitProposal submits immutable Skill content without activating it.
func (s *Skills) SubmitProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	if err := proposal.Validate(); err != nil {
		return skills.ProposalRef{}, err
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	ref, identities, err := s.proposals.SubmitProposal(ctx, root, proposal)
	s.publishSkillMutation(identities)
	if err != nil {
		return ref, err
	}
	if err := ref.Validate(); err != nil {
		return skills.ProposalRef{}, fmt.Errorf("workspace: submitted proposal reference is invalid: %w", err)
	}
	if ref.Scope != proposal.Scope || ref.Name != proposal.Name {
		return skills.ProposalRef{}, fmt.Errorf(
			"workspace: submitted proposal reference %s/%s does not acknowledge %s/%s",
			ref.Scope, ref.Name, proposal.Scope, proposal.Name,
		)
	}
	return ref, nil
}

// Proposals returns the current immutable Skill proposals visible from cwd,
// ordered by scope and name.
func (s *Skills) Proposals(ctx context.Context, cwd string) ([]skills.ProposalReview, error) {
	root, err := s.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	proposals, err := s.proposals.ListProposals(ctx, root)
	if err != nil {
		return nil, err
	}
	if len(proposals) > 2*skills.MaxPendingProposalsPerScope {
		return nil, fmt.Errorf("%w: review catalog contains %d proposals", skills.ErrProposalQueueFull, len(proposals))
	}
	for index, proposal := range proposals {
		if err := proposal.Validate(); err != nil {
			return nil, fmt.Errorf("workspace: Skill proposal %d is invalid: %w", index+1, err)
		}
	}
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

func validateSkillSummary(summary SkillSummary) error {
	if err := summary.Scope.Validate(); err != nil {
		return err
	}
	return (skills.Entry{
		Name: summary.Name, Description: summary.Description, Lifecycle: skills.Active,
	}).Validate()
}

// ApproveProposal accepts a Skill proposal into its target library.
func (s *Skills) ApproveProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if err := ref.Validate(); err != nil {
		return err
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
	if err := ref.Validate(); err != nil {
		return err
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
	s.observations.Accept(AuthoredChange{Resource: AuthoredSkills, Identities: identities})
	s.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
}
