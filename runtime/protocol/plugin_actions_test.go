package protocol

import "testing"

func TestPluginRenameRequiresTitleAndForbidsOtherSessionChanges(t *testing.T) {
	prefix := `{"installationId":"940ac827-b431-455b-af4b-e3a170bcfda0","digest":"1111111111111111111111111111111111111111111111111111111111111111","actionId":"rename","update":`
	for _, update := range []string{
		`{"sessionId":"ses_1","expectedRevision":7}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":null}`,
		`{"sessionId":"ses_1","expectedRevision":0,"title":"Reviewed"}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":"Reviewed","favorite":false}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":"Reviewed","isolated":false}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":"Reviewed","workspace":{"path":"/other"}}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":"Reviewed","provider":"other","model":"other"}`,
		`{"sessionId":"ses_1","expectedRevision":7,"title":"Reviewed","reasoningEffort":"other"}`,
	} {
		var request RenamePluginSessionRequest
		if err := DecodeRequest([]byte(prefix+update+`}`), &request); err == nil {
			t.Fatalf("accepted %s", update)
		}
	}
	var request RenamePluginSessionRequest
	if err := DecodeRequest([]byte(prefix+`{"sessionId":"ses_1","expectedRevision":7,"title":""}}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.Update.Title != "" {
		t.Fatal("explicit title edit lost")
	}
}
