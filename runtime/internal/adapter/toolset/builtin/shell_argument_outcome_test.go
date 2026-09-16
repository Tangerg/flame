package builtin

import (
	"errors"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

// Rejecting the model's arguments happens before any command is launched, so the
// call definitely did not happen. Left unclassified, the Host reads the error as
// an operation whose durable outcome it cannot prove and settles the whole Run
// tree as lost — a contradictory flag pair ended a real Session this way.
func TestShellArgumentRejectionFailsTheCallNotTheRun(t *testing.T) {
	shells := exec.NewShells(nil, false)
	cleanupShells(t, shells)

	for _, testCase := range []struct {
		name      string
		tool      string
		arguments string
	}{
		{
			name:      "background flags contradict",
			tool:      tool.Shell,
			arguments: `{"command":"true","description":"Run tests","run_in_background":true,"auto_background_after_seconds":5}`,
		},
		{
			name:      "description missing",
			tool:      tool.Shell,
			arguments: `{"command":"true","description":"   "}`,
		},
		{
			name:      "timeout without wait",
			tool:      tool.ReadShellOutput,
			arguments: `{"shell_id":"shell_1","timeout_millis":10}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := callTextTool(t.Context(), shellTool(t, shells, testCase.tool), testCase.arguments)
			if err == nil {
				t.Fatalf("%s accepted %s", testCase.tool, testCase.arguments)
			}
			var failure *toolcontract.Failure
			if !errors.As(err, &failure) {
				t.Fatalf("%s returned an unclassified error (%v); the Host settles that as an "+
					"operation of unknown outcome and loses the Run tree", testCase.tool, err)
			}
			if failure.Kind() != toolcontract.FailureKindFailed {
				t.Fatalf("failure kind = %q, want %q: nothing was refused permission, the call did not happen",
					failure.Kind(), toolcontract.FailureKindFailed)
			}
		})
	}
}
