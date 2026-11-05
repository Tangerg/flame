package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/skill"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

type fakeSkillProposals struct {
	root       string
	proposal   skills.Proposal
	list       []skills.ProposalReview
	approved   []skills.ProposalRef
	rejected   []skills.ProposalRef
	submitErr  error
	approveErr error
	rejectErr  error
}

func (f *fakeSkillProposals) SubmitProposal(_ context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, error) {
	f.root = projectRoot
	f.proposal = proposal
	if f.submitErr != nil {
		return skills.ProposalRef{}, f.submitErr
	}
	return skills.CommandRef(proposal.Scope, proposal.Name, fmt.Sprintf("%x", sha256.Sum256([]byte(proposal.Instructions)))), nil
}

func (f *fakeSkillProposals) ListProposals(_ context.Context, projectRoot string) ([]skills.ProposalReview, error) {
	f.root = projectRoot
	return slices.Clone(f.list), nil
}

func (f *fakeSkillProposals) ApproveProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error) {
	f.root = projectRoot
	if f.approveErr != nil {
		return nil, f.approveErr
	}
	f.approved = append(f.approved, ref)
	return []string{"/repo/.flame/skills/run-tests/SKILL.md"}, nil
}

func (f *fakeSkillProposals) RejectProposal(_ context.Context, projectRoot string, ref skills.ProposalRef) error {
	f.root = projectRoot
	if f.rejectErr != nil {
		return f.rejectErr
	}
	f.rejected = append(f.rejected, ref)
	return nil
}

func TestSkillProposalsResolveWorkspaceAndDelegate(t *testing.T) {
	proposal := skills.Proposal{Origin: skills.ProposalOriginRequested, Scope: skills.ScopeProject, Name: "run-tests", Description: "Run the project tests when verification is requested.", Instructions: "Run the tests."}
	ref := skills.CommandRef(proposal.Scope, proposal.Name, fmt.Sprintf("%x", sha256.Sum256([]byte(proposal.Instructions))))
	fake := &fakeSkillProposals{list: []skills.ProposalReview{{Origin: skills.ProposalOriginRequested, Ref: ref, Description: proposal.Description, Instructions: proposal.Instructions}}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, fake, nil, nil)

	gotRef, err := c.SubmitProposal(t.Context(), "/repo", proposal)
	if err != nil || gotRef != ref {
		t.Fatalf("SubmitProposal = %+v, %v; want %+v", gotRef, err, ref)
	}
	got, err := c.Proposals(t.Context(), "/repo")
	if err != nil || len(got) != 1 || got[0].Ref != ref {
		t.Fatalf("Proposals = %+v, %v", got, err)
	}
	if err := c.ApproveProposal(t.Context(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if err := c.RejectProposal(t.Context(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if fake.root != "/repo" || fake.proposal != proposal {
		t.Fatalf("delegated root/proposal = %q, %+v", fake.root, fake.proposal)
	}
	if len(fake.approved) != 1 || fake.approved[0] != ref {
		t.Fatalf("approved = %+v", fake.approved)
	}
	if len(fake.rejected) != 1 || fake.rejected[0] != ref {
		t.Fatalf("rejected = %+v", fake.rejected)
	}
}

func TestSkillProposalsOwnCurrentSlotOrder(t *testing.T) {
	projectAlpha := skills.CommandRef(skills.ScopeProject, "alpha", fmt.Sprintf("%x", sha256.Sum256([]byte("project alpha"))))
	projectZeta := skills.CommandRef(skills.ScopeProject, "zeta", fmt.Sprintf("%x", sha256.Sum256([]byte("project zeta"))))
	userAlpha := skills.CommandRef(skills.ScopeUser, "alpha", fmt.Sprintf("%x", sha256.Sum256([]byte("user alpha"))))
	fake := &fakeSkillProposals{list: []skills.ProposalReview{
		validProposalReview(userAlpha),
		validProposalReview(projectZeta),
		validProposalReview(projectAlpha),
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, fake, nil, nil)

	got, err := c.Proposals(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Ref != projectAlpha || got[1].Ref != projectZeta || got[2].Ref != userAlpha {
		t.Fatalf("Proposals = %+v, want project/alpha, project/zeta, user/alpha", got)
	}
	got[0].Ref = userAlpha
	next, err := c.Proposals(t.Context(), "/repo")
	if err != nil || len(next) != 3 || next[0].Ref != projectAlpha {
		t.Fatalf("Proposals after caller reused result = (%+v, %v)", next, err)
	}
}

func TestSkillProposalsRejectDuplicateOriginalCommand(t *testing.T) {
	first := skills.CommandRef(skills.ScopeProject, "review", fmt.Sprintf("%x", sha256.Sum256([]byte("first"))))
	second := first
	fake := &fakeSkillProposals{list: []skills.ProposalReview{
		validProposalReview(first), validProposalReview(second),
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, fake, nil, nil)

	if _, err := c.Proposals(t.Context(), "/repo"); err == nil {
		t.Fatal("Proposals accepted a duplicate original proposal command")
	}
}

func TestSkillProposalsRejectInvalidOrUnboundedCatalog(t *testing.T) {
	ref := skills.CommandRef(skills.ScopeProject, "review", fmt.Sprintf("%x", sha256.Sum256([]byte("proposal"))))
	invalid := validProposalReview(ref)
	invalid.Instructions = ""
	for name, list := range map[string][]skills.ProposalReview{
		"invalid row": {invalid},
		"capacity":    make([]skills.ProposalReview, 2*plugin.MaxInstallations*skill.MaxPendingProposalsPerScope+1),
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeSkillProposals{list: list}
			c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, fake, nil, nil)
			if _, err := c.Proposals(t.Context(), "/repo"); err == nil {
				t.Fatal("Proposals error = nil, want rejected catalog")
			}
		})
	}
}

func TestSkillProposalMutationsValidateIdentity(t *testing.T) {
	proposal := skills.Proposal{
		Origin: skills.ProposalOriginRequested,
		Scope:  skills.ScopeProject, Name: "review",
		Description: "Review the current project changes.", Instructions: "Inspect the diff.",
	}
	invalidProposal := proposal
	invalidProposal.Instructions = ""
	unreached := &fakeSkillProposals{}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, unreached, nil, nil)
	if _, err := c.SubmitProposal(t.Context(), "/repo", invalidProposal); err == nil {
		t.Fatal("SubmitProposal accepted invalid content")
	}
	if unreached.root != "" {
		t.Fatal("invalid proposal reached store")
	}

	invalid := skills.ProposalRef{Scope: skills.ScopeProject, Name: "review", Revision: "invalid"}
	fake := &fakeSkillProposals{}
	c = newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, fake, nil, nil)
	if err := c.ApproveProposal(t.Context(), "/repo", invalid); err == nil {
		t.Fatal("ApproveProposal accepted invalid reference")
	}
	if err := c.RejectProposal(t.Context(), "/repo", invalid); err == nil {
		t.Fatal("RejectProposal accepted invalid reference")
	}
	if len(fake.approved) != 0 || len(fake.rejected) != 0 {
		t.Fatal("invalid proposal reference reached store")
	}
}

func validProposalReview(ref skills.ProposalRef) skills.ProposalReview {
	return skills.ProposalReview{
		Origin: skills.ProposalOriginRequested,
		Ref:    ref, Description: "Review this reusable Skill workflow.",
		Instructions: "Follow the reusable workflow.",
	}
}
