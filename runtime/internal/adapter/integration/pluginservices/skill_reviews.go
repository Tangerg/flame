package pluginservices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/skill"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	sdk "github.com/Tangerg/go-sdk/mcp"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

type skillPublisher interface {
	Library(context.Context, string, skills.Scope) (string, error)
	Preimage(context.Context, string, skills.Proposal) (string, error)
	Publish(context.Context, string, skills.Proposal, string, string) ([]string, error)
}
type SkillGovernance struct {
	calls    *CommandCalls
	files    skillPublisher
	contract *portable.Capability
}

func NewSkillGovernance(calls *CommandCalls, files skillPublisher) (*SkillGovernance, error) {
	contract, err := skilllearning.Contract()
	if err != nil {
		return nil, err
	}
	if calls == nil || files == nil {
		return nil, errors.New("skill reviews: commands and publication are required")
	}
	return &SkillGovernance{calls: calls, files: files, contract: contract}, nil
}

func (s *SkillGovernance) libraryID(ctx context.Context, root string, scope skills.Scope, source plugins.ContributionBinding) (string, error) {
	path, err := s.files.Library(ctx, root, scope)
	if err != nil {
		return "", err
	}
	// Release replacement may reopen the same installation-private database.
	// Pending names from the earlier release must remain independently owned.
	digest := sha256.Sum256([]byte(source.Digest.String() + "\x00" + source.ContributionID.String() + "\x00" + string(scope) + "\x00" + path))
	return hex.EncodeToString(digest[:]), nil
}

func (s *SkillGovernance) selected(ctx context.Context) (plugins.ContributionBinding, error) {
	c := s.calls
	binding, found, err := plugins.SelectedContribution(ctx, c.framework, c.installations, c.selections, s.contract)
	if err == nil && !found {
		return binding, plugin.ErrUnavailable
	}
	return binding, err
}
func (s *SkillGovernance) SubmitProposal(ctx context.Context, root string, p skills.Proposal) (skills.ProposalRef, error) {
	var zero skills.ProposalRef
	if err := p.Validate(); err != nil {
		return zero, err
	}
	method, err := skilllearning.ProposeMethod()
	if err != nil {
		return zero, err
	}
	ctx = operation.Bind(ctx, method.Name(), operation.Identity(ctx))
	original := operation.Identity(ctx)
	draft := skilllearning.Draft{Scope: skill.Scope(p.Scope), Skill: skilllearning.Skill{Name: p.Name, Description: p.Description, Instructions: p.Instructions}, Origin: skill.ProposalOrigin(p.Origin), SourceSession: p.SourceSession, Revises: p.Revises}
	record, err := s.calls.routing.Get(ctx, original)
	var binding plugins.ContributionBinding
	switch {
	case err == nil:
		var retained skilllearning.Draft
		if err := json.Unmarshal(record.Snapshot().Input.JSON(), &retained); err != nil {
			return zero, err
		}
		draft.Previous = retained.Previous
		binding, err = s.calls.original(ctx, record.Snapshot())
		if err != nil {
			return zero, err
		}
	case errors.Is(err, plugin.ErrNotFound), errors.Is(err, plugin.ErrCommandUnconfirmed):
		binding, err = s.selected(ctx)
		if err != nil {
			return zero, err
		}
		draft.Previous, err = s.files.Preimage(ctx, root, p)
		if err != nil {
			return zero, err
		}
	default:
		return zero, err
	}
	draft.Library, err = s.libraryID(ctx, root, p.Scope, binding)
	if err != nil {
		return zero, err
	}
	_, err = invokeResolvedCommand(ctx, s.calls, method.Name(), draft, func(ref skilllearning.ReviewRef) error {
		if ref.Command != original {
			return skills.ErrProposalChanged
		}
		return nil
	}, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, nil)
	if err != nil {
		return zero, reviewError(err)
	}
	s.changed()
	return skills.CommandRef(p.Scope, p.Name, original), nil
}

type reviewDocument struct {
	draft  skilllearning.Draft
	source plugins.ContributionBinding
	record plugin.CommandBinding
}

func (s *SkillGovernance) document(ctx context.Context, root, id string) (reviewDocument, error) {
	var doc reviewDocument
	original, err := s.calls.routing.Get(ctx, id)
	if errors.Is(err, plugin.ErrNotFound) {
		return doc, skills.ErrNotFound
	}
	if err != nil {
		return doc, err
	}
	record := original.Snapshot()
	if record.Contract != s.contract.Identity() {
		return doc, skills.ErrProposalChanged
	}
	doc.source, err = s.calls.original(ctx, record)
	if err != nil {
		return doc, err
	}
	doc.draft, err = s.retainedDraft(ctx, record)
	if err != nil {
		return doc, err
	}
	p := doc.proposal()
	if err := p.Validate(); err != nil {
		return doc, err
	}
	library, err := s.libraryID(ctx, root, p.Scope, doc.source)
	if err != nil {
		return doc, err
	}
	if doc.draft.Library != library {
		return doc, skills.ErrProposalChanged
	}
	doc.source.Activation = &record.Activation
	doc.record = record
	return doc, nil
}
func (s *SkillGovernance) retainedDraft(ctx context.Context, record plugin.CommandBinding) (skilllearning.Draft, error) {
	var draft skilllearning.Draft
	propose, e1 := skilllearning.ProposeMethod()
	mine, e2 := skilllearning.MineMethod()
	if err := errors.Join(e1, e2); err != nil {
		return draft, err
	}
	switch record.Method {
	case propose.Name():
		err := json.Unmarshal(record.Input.JSON(), &draft, json.RejectUnknownMembers(true))
		return draft, err
	case mine.Name():
		var run skilllearning.Run
		if err := json.Unmarshal(record.Input.JSON(), &run, json.RejectUnknownMembers(true)); err != nil {
			return draft, err
		}
		result, err := originalSkillReceipt[skilllearning.MineResult](ctx, s.calls, record)
		if err != nil {
			return draft, err
		}
		if result.Proposal == nil {
			return draft, skills.ErrProposalChanged
		}
		return result.Proposal.Draft(run), nil
	default:
		return draft, skills.ErrProposalChanged
	}
}

func (d reviewDocument) proposal() skills.Proposal {
	v := d.draft
	return skills.Proposal{Scope: skills.Scope(v.Scope), Name: v.Skill.Name, Description: v.Skill.Description, Instructions: v.Skill.Instructions, Origin: skills.ProposalOrigin(v.Origin), SourceSession: v.SourceSession, Revises: v.Revises}
}
func (d reviewDocument) review() skills.ProposalReview {
	p := d.proposal()
	return skills.ProposalReview{Ref: skills.CommandRef(p.Scope, p.Name, d.record.ID), Description: p.Description, Instructions: p.Instructions, Origin: p.Origin, SourceSession: p.SourceSession, Revises: p.Revises}
}

func (s *SkillGovernance) ListProposals(ctx context.Context, root string) ([]skills.ProposalReview, error) {
	all, err := s.calls.installations.List(ctx)
	if err != nil {
		return nil, err
	}
	var result []skills.ProposalReview
	for _, snapshot := range all {
		for _, contribution := range snapshot.Selected.Declaration().Contributions {
			if contribution.Contract != s.contract.Identity() || !snapshot.Installation.ServerEnabled(contribution.Server) {
				continue
			}
			source, err := s.calls.original(ctx, plugin.CommandBinding{Provider: plugin.Dependency{InstallationID: snapshot.Installation.ID(), Digest: snapshot.Selected.Digest()}, Server: contribution.Server, Export: contribution.Export, Contribution: contribution.ID, Contract: contribution.Contract})
			if err != nil {
				return nil, err
			}
			items, err := s.list(ctx, root, source)
			if err != nil {
				return nil, err
			}
			result = append(result, items...)
		}
	}
	return result, nil
}
func (s *SkillGovernance) list(ctx context.Context, root string, source plugins.ContributionBinding) ([]skills.ProposalReview, error) {
	borrowed, err := borrowContribution(ctx, s.calls.pool, source)
	if err != nil {
		return nil, err
	}
	defer borrowed.Close()
	method, err := skilllearning.QueueMethod()
	if err != nil {
		return nil, err
	}
	user, err := s.libraryID(ctx, root, skills.ScopeUser, source)
	if err != nil {
		return nil, err
	}
	project, err := s.libraryID(ctx, root, skills.ScopeProject, source)
	if err != nil {
		return nil, err
	}
	result, err := query[skilllearning.Libraries, skilllearning.Queue](borrowed.Context(), source, borrowed.Session(), method.Name(), skilllearning.Libraries{User: user, Project: project}, sdk.ParamsBase{})
	if err != nil {
		return nil, err
	}
	if len(result.Proposals) > 2*skill.MaxPendingProposalsPerScope {
		return nil, skills.ErrProposalQueueFull
	}
	items := make([]skills.ProposalReview, 0, len(result.Proposals))
	for _, ref := range result.Proposals {
		doc, err := s.document(ctx, root, ref.Command)
		if errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrCommandRetired) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !source.SameContribution(doc.source) {
			return nil, skills.ErrProposalChanged
		}
		items = append(items, doc.review())
	}
	return items, nil
}

func (s *SkillGovernance) admitted(ctx context.Context, root string, ref skills.ProposalRef) (reviewDocument, error) {
	doc, err := s.document(ctx, root, ref.Revision)
	if err != nil {
		return doc, err
	}
	if doc.review().Ref != ref {
		return doc, skills.ErrProposalChanged
	}
	caller, err := operation.Capture(ctx, portable.Target{Scope: extension.ScopeInstallation, Reference: doc.source.InstallationID.String()})
	if err != nil || caller.Origin != extension.Human {
		return doc, plugin.ErrInvalid
	}
	if _, managed := operation.ActorFrom(ctx); managed {
		return doc, plugin.ErrInvalid
	}
	return doc, nil
}

func reviewMutation[P, R any](ctx context.Context, s *SkillGovernance, doc reviewDocument, method *qimumcp.Method[P, R], input P) (R, error) {
	child := operation.Bind(ctx, method.Name(), operation.Identity(ctx))
	reply, err := invokeResolvedCommand[P, R](child, s.calls, method.Name(), input, nil, func(ctx context.Context) (plugins.ContributionBinding, error) { return doc.source, nil }, nil)
	return reply.value, reviewError(err)
}
func (s *SkillGovernance) RejectProposal(ctx context.Context, root string, ref skills.ProposalRef) error {
	doc, err := s.admitted(ctx, root, ref)
	if err != nil {
		return err
	}
	method, err := skilllearning.RejectMethod()
	if err != nil {
		return err
	}
	_, err = reviewMutation(ctx, s, doc, method, skilllearning.ReviewRef{Command: ref.Revision})
	if err == nil {
		s.changed()
	}
	return err
}
func (s *SkillGovernance) ApproveProposal(ctx context.Context, root string, ref skills.ProposalRef) ([]string, error) {
	doc, err := s.admitted(ctx, root, ref)
	if err != nil {
		return nil, err
	}
	method, err := skilllearning.ApproveMethod()
	if err != nil {
		return nil, err
	}
	reference := skilllearning.ReviewRef{Command: ref.Revision}
	permit, err := reviewMutation(ctx, s, doc, method, reference)
	if err != nil {
		return nil, err
	}
	approval, err := s.calls.routing.Get(ctx, permit.Decision)
	if err != nil {
		return nil, err
	}
	record := approval.Snapshot()
	var original skilllearning.ReviewRef
	if err := json.Unmarshal(record.Input.JSON(), &original); err != nil {
		return nil, err
	}
	if record.Method != method.Name() || record.Provider != doc.record.Provider || record.Contract != doc.record.Contract || record.Contribution != doc.record.Contribution || record.Invocation.Origin != extension.Human || original != reference {
		return nil, skills.ErrProposalChanged
	}
	var changed []string
	publishErr := s.calls.framework.Admission().Admit(ctx, func(ctx context.Context) error {
		if err := s.calls.admitDispatch(ctx, doc.record); err != nil {
			return err
		}
		if err := s.calls.admitDispatch(ctx, record); err != nil {
			return err
		}
		var err error
		changed, err = s.files.Publish(ctx, root, doc.proposal(), doc.draft.Previous, permit.Decision)
		return err
	})
	if publishErr != nil && !errors.Is(publishErr, skills.ErrProposalChanged) && !errors.Is(publishErr, skills.ErrConflict) && !errors.Is(publishErr, skills.ErrLibraryCapacity) {
		return changed, publishErr
	}
	confirm, err := skilllearning.ConfirmMethod()
	if err != nil {
		return changed, err
	}
	_, err = reviewMutation(ctx, s, doc, confirm, skilllearning.Publication{Proposal: reference, Permit: permit, Published: publishErr == nil})
	if err == nil {
		s.changed()
	}
	return changed, errors.Join(publishErr, err)
}
func (s *SkillGovernance) changed() {
	s.calls.publish.Notify(invalidation.Notice{Resource: invalidation.Skills})
}
func reviewError(err error) error {
	if refusal, ok := errors.AsType[*portable.Refusal](err); ok && refusal.Code == skilllearning.StaleReview {
		return errors.Join(skills.ErrProposalChanged, err)
	}
	return err
}
