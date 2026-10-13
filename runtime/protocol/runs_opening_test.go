package protocol

import "testing"

func TestStartRunTargetsExactlyOneCompleteOpening(t *testing.T) {
	input := []ContentBlock{{Type: ContentBlockText, Text: "start"}}
	opening := &RunSessionOpening{SessionID: "ses_captured", RunID: "run_captured", Title: "captured", Workspace: WorkspaceRef{Path: "/work"}}
	for _, request := range []StartRunRequest{{SessionID: "ses_existing", Input: input}, {NewSession: opening, Input: input}} {
		if err := ValidateWireTree(request); err != nil {
			t.Fatalf("valid target refused: %v", err)
		}
	}
	for _, request := range []StartRunRequest{
		{Input: input},
		{SessionID: "ses_existing", NewSession: opening, Input: input},
		{NewSession: &RunSessionOpening{RunID: "run_captured", Workspace: WorkspaceRef{Path: "/work"}}, Input: input},
		{NewSession: &RunSessionOpening{SessionID: "ses_captured", Workspace: WorkspaceRef{Path: "/work"}}, Input: input},
	} {
		if err := ValidateWireTree(request); err == nil {
			t.Fatalf("incomplete or competing targets admitted: %+v", request)
		}
	}
}
