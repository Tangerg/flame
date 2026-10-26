package protocol

import "testing"

func TestStartRunTargetsExactlyOneCompleteOpening(t *testing.T) {
	input := []ContentBlock{{Type: ContentBlockText, Text: "start"}}
	opening := &RunSessionOpening{SessionID: "ses_captured", RunID: "run_captured", Title: "captured", Workspace: &WorkspaceRef{Path: "/work"}}
	defaultOpening := &RunSessionOpening{SessionID: "ses_default", RunID: "run_default", Title: "default workspace"}
	for _, request := range []StartRunRequest{{SessionID: "ses_existing", Input: input}, {NewSession: opening, Input: input}, {NewSession: defaultOpening, Input: input}} {
		if err := ValidateWireTree(request); err != nil {
			t.Fatalf("valid target refused: %v", err)
		}
		acceptance := GetRunAcceptanceRequest{Namespace: "idp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Key: "opening", Operation: RunCommandStart, Request: &request}
		if err := ValidateWireTree(acceptance); err != nil {
			t.Fatalf("acceptance query cannot represent the original opening: %v", err)
		}
	}
	for _, request := range []StartRunRequest{
		{Input: input},
		{SessionID: "ses_existing", NewSession: opening, Input: input},
		{NewSession: &RunSessionOpening{RunID: "run_captured", Workspace: &WorkspaceRef{Path: "/work"}}, Input: input},
		{NewSession: &RunSessionOpening{SessionID: "ses_captured", Workspace: &WorkspaceRef{Path: "/work"}}, Input: input},
		{NewSession: &RunSessionOpening{SessionID: "ses_captured", RunID: "run_captured", Workspace: &WorkspaceRef{}}, Input: input},
	} {
		if err := ValidateWireTree(request); err == nil {
			t.Fatalf("incomplete or competing targets admitted: %+v", request)
		}
	}
}
