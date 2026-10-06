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
