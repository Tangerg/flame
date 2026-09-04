package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestProposalReferenceBindsExactContent(t *testing.T) {
	content := []byte("proposal bytes")
	ref := NewProposalRef(ScopeProject, "reviewed-skill", content)
	if err := ref.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !ref.Matches(content) || ref.Matches([]byte("different bytes")) {
		t.Fatal("ProposalRef did not bind the exact proposal content")
	}
	uppercase := ref
	uppercase.Revision = strings.ToUpper(uppercase.Revision)
	if err := uppercase.Validate(); err == nil {
		t.Fatal("ProposalRef accepted a non-canonical uppercase revision")
	}
}

func TestProposalValidatesMeaningAndSafety(t *testing.T) {
	safe := Proposal{
		Scope: ScopeUser, Name: "safe-skill",
		Description:  "A sufficiently descriptive Skill proposal.",
		Instructions: "Inspect the requested files and report the result.",
		Origin:       ProposalOriginRequested,
	}
	if err := safe.Validate(); err != nil {
		t.Fatalf("safe Validate() error = %v", err)
	}
	if issue := safe.SafetyIssue(); issue != ProposalSafe {
		t.Fatalf("safe SafetyIssue() = %v, want ProposalSafe", issue)
	}

	dangerous := safe
	dangerous.Instructions = "run rm -rf /"
	if issue := dangerous.SafetyIssue(); issue != ProposalDangerousInstruction {
		t.Fatalf("dangerous SafetyIssue() = %v, want ProposalDangerousInstruction", issue)
	}

	invalidRef := NewProposalRef(Scope("other"), "safe-skill", nil)
	if err := invalidRef.Validate(); err == nil {
		t.Fatal("invalid proposal scope passed validation")
	}

	oversized := safe
	oversized.Instructions = strings.Repeat("x", MaxAuthoredSkillDocumentBytes+1)
	if err := oversized.Validate(); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized Validate() error = %v, want ErrDocumentTooLarge", err)
	}
}

func TestEntryValidatesManagedLibraryRows(t *testing.T) {
	valid := Entry{Name: "review", Description: "Review the current project changes.", Lifecycle: Active}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Entry.Validate() error = %v", err)
	}

	for name, entry := range map[string]Entry{
		"frontmatter": {Name: "review", Lifecycle: Active},
		"lifecycle":   {Name: "review", Description: valid.Description, Lifecycle: Lifecycle("unknown")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := entry.Validate(); err == nil {
				t.Fatal("Entry.Validate() error = nil, want invalid row")
			}
		})
	}
}

func TestProposalReviewValidatesReferenceAndContent(t *testing.T) {
	ref := NewProposalRef(ScopeProject, "review", []byte("proposal"))
	valid := ProposalReview{
		Ref: ref, Description: "Review the current project changes.",
		Instructions: "Inspect the diff and report actionable findings.",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ProposalReview.Validate() error = %v", err)
	}

	invalidRef := valid
	invalidRef.Ref.Revision = "invalid"
	invalidContent := valid
	invalidContent.Instructions = ""
	for name, review := range map[string]ProposalReview{
		"reference": invalidRef,
		"content":   invalidContent,
	} {
		t.Run(name, func(t *testing.T) {
			if err := review.Validate(); err == nil {
				t.Fatal("ProposalReview.Validate() error = nil, want invalid review")
			}
		})
	}
}
