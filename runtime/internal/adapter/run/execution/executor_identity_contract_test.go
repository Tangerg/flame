package execution

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/agent"
)

func TestRuntimeExecutorIdentityPolicyMatchesAgentFrameworkPort(t *testing.T) {
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "process:root_1", SessionID: "ses_1", Payload: []byte("opaque checkpoint"),
		BuildID: runs.BuildFromSHA256(sha256.Sum256([]byte(t.Name()))).String(),
	}
	interrupt := runs.Interrupt{
		Kind: run.InterruptQuestion,
		Question: &runs.QuestionPrompt{
			ToolName: "ask_user", Arguments: "{}", Fields: []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
		},
	}
	values := []string{
		"process:root_1",
		strings.Repeat("x", runs.MaximumIdentityBytes),
		"",
		"process root",
		"process/root",
		"process\u200broot",
		string([]byte{0xff}),
		strings.Repeat("x", runs.MaximumIdentityBytes+1),
	}
	for _, value := range values {
		runtimeErr := runs.ValidateMember(value)
		_, frameworkErr := agent.ParseProcessID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("member policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
		_, runtimeErr = runs.NewTreeInterrupted(checkpoint, []runs.MemberInterruption{{
			MemberID: checkpoint.RootMemberID, RequestID: value, Interrupt: interrupt,
		}})
		_, frameworkErr = agent.ParseWaitID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("request policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
		runtimeErr = runs.ValidateInvocation(value)
		_, frameworkErr = agent.ParseEffectID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("effect policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
	}
}
