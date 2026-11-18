package builtin

import (
	json "encoding/json/v2"

	"strings"
	"testing"
)

func TestNormalizedResultEncodingRetainsPresenceAndZeroCounts(t *testing.T) {
	zero := 0
	encoded, err := json.Marshal(CommandResult{ExitCode: &zero})
	if err != nil || !strings.Contains(string(encoded), `"exitCode":0`) {
		t.Fatalf("finished command = %s, %v", encoded, err)
	}
	encoded, err = json.Marshal(CommandResult{})
	if err != nil || strings.Contains(string(encoded), "exitCode") {
		t.Fatalf("unfinished command = %s, %v", encoded, err)
	}
	encoded, err = json.Marshal(PatchResult{Changes: []AppliedChange{{Path: "next.go", Status: Moved, From: "old.go"}}})
	if err != nil || !strings.Contains(string(encoded), `"added":0`) || !strings.Contains(string(encoded), `"removed":0`) {
		t.Fatalf("pure move = %s, %v", encoded, err)
	}
}
