package workspace

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

const testSkillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestProposalReferencePreservesImmutableReviewIdentity(t *testing.T) {
	proposal := SkillProposal{
		Name: "release-checks", Revision: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Scope: protocol.SkillScopeUser, Description: "Review releases consistently.", Instructions: "Run every release gate.",
		Origin: protocol.SkillProposalOriginRequested,
	}
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}
	reference, err := proposal.Reference("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if reference.Name != proposal.Name || reference.Revision != proposal.Revision || reference.Scope != proposal.Scope {
		t.Fatalf("reference = %+v", reference)
	}
	if proposal.Key() != "user/release-checks@0123456789ab" {
		t.Fatalf("proposal key = %q", proposal.Key())
	}
	proposal.Revision = "changed"
	if err := proposal.Validate(); err == nil {
		t.Fatal("malformed proposal revision was accepted")
	}
}

func TestSkillClosedVocabulariesRejectUnknownValues(t *testing.T) {
	if err := (protocol.Skill{Name: "review", Scope: protocol.SkillScope("global")}).ValidateWire(); err == nil {
		t.Fatal("unknown scope was accepted")
	}
	if err := (protocol.ManagedSkill{Name: "review", Lifecycle: protocol.SkillLifecycle("stale")}).ValidateWire(); err == nil {
		t.Fatal("unknown lifecycle was accepted")
	}
}

func TestManagedSkillValidatesLifecycleAcknowledgement(t *testing.T) {
	catalog := []protocol.ManagedSkill{{Name: "review", Lifecycle: protocol.SkillLifecycleActive}}
	if err := ValidateSkillLifecycleAcknowledgement(catalog, "review", protocol.SkillLifecycleActive); err != nil {
		t.Fatalf("active lifecycle acknowledgement: %v", err)
	}
	if err := ValidateSkillLifecycleAcknowledgement(catalog, "review", protocol.SkillLifecycleArchived); err == nil {
		t.Fatal("accepted unchanged lifecycle")
	}
	if err := ValidateSkillLifecycleAcknowledgement(catalog, "missing", protocol.SkillLifecycleActive); err == nil {
		t.Fatal("accepted missing managed skill")
	}
}

func TestProposalReferenceValidatesDecisionAcknowledgement(t *testing.T) {
	reference := SkillProposalReference{Workspace: "/workspace", Name: "release-checks", Revision: testSkillRevision, Scope: protocol.SkillScopeUser}
	decided := SkillProposal{
		Name: reference.Name, Revision: reference.Revision, Scope: reference.Scope,
		Description: "Release safely", Instructions: "Run every gate.",
	}
	if err := reference.ValidateDecisionAcknowledgement([]SkillProposal{decided}); err == nil {
		t.Fatal("accepted the reviewed proposal as still pending")
	}
	newRevision := decided
	newRevision.Revision = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if err := reference.ValidateDecisionAcknowledgement([]SkillProposal{newRevision}); err != nil {
		t.Fatalf("new proposal revision: %v", err)
	}
}
