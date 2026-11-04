package execution

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func TestRuntimeExecutorIdentityPolicyMatchesAgentFrameworkPort(t *testing.T) {
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "process:root_1", SessionID: "ses_1", Payload: []byte("opaque checkpoint"),
		BuildID: runs.BuildFromSHA256(sha256.Sum256([]byte(t.Name()))).String(),
	}
	interrupt := runs.QuestionPrompt{
		ToolName: "ask_user", Arguments: "{}", Fields: []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
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
		_, frameworkErr := scoperun.ParseProcessID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("member policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
		_, runtimeErr = runs.NewTreeInterrupted(checkpoint, []runs.MemberInterruption{{
			MemberID: checkpoint.RootMemberID, RequestID: value, Interrupt: interrupt,
		}})
		_, frameworkErr = scoperun.ParseWaitKey(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("request policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
	}
}

func TestScopeEffectIdentityProjectsToBoundedProductCall(t *testing.T) {
	process, err := scoperun.ParseProcessID(strings.Repeat("a", runs.MaximumIdentityBytes) + "/" + strings.Repeat("b", runs.MaximumIdentityBytes))
	if err != nil {
		t.Fatal(err)
	}
	effect := scoperun.EffectID{Process: process, Step: 1, Index: 0}
	invocation, err := modelInvocationIDFrom(effect, 1)
	if err != nil || runs.ValidateInvocation(invocation.String()) != nil {
		t.Fatalf("model invocation %q: %v", invocation, err)
	}
	if parent := effectInvocationParent(effect); runs.ValidateInvocation(parent) != nil {
		t.Fatalf("effect parent is not a bounded caller identity: %q", parent)
	}
	other, err := modelInvocationIDFrom(effect, 2)
	if err != nil || other == invocation {
		t.Fatalf("model sequence lost its separate identity: %q, %v", other, err)
	}
}
