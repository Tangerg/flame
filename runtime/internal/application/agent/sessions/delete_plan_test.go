package sessions

import "testing"

func TestDeletePlanOwnsCanonicalSessionIdentity(t *testing.T) {
	deletion, err := NewDeletePlan("ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if deletion.SessionID() != "ses_1" {
		t.Fatalf("SessionID = %q, want ses_1", deletion.SessionID())
	}
	if err := deletion.Validate(); err != nil {
		t.Fatalf("DeletePlan became invalid: %v", err)
	}
}

func TestDeletePlanRejectsInvalidSessionIdentity(t *testing.T) {
	for _, sessionID := range []string{"", "ses bad"} {
		if _, err := NewDeletePlan(sessionID); err == nil {
			t.Fatalf("NewDeletePlan(%q) accepted an invalid identity", sessionID)
		}
	}
	if err := (DeletePlan{}).Validate(); err == nil {
		t.Fatal("zero DeletePlan is valid")
	}
}
