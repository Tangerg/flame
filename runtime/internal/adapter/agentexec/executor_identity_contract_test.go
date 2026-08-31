package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/agent"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestRuntimeExecutorIdentityPolicyMatchesAgentFrameworkPort(t *testing.T) {
	values := []string{
		"process:root_1",
		strings.Repeat("x", runtimeidentity.MaximumExecutorIdentityBytes),
		"",
		"process root",
		"process/root",
		"process\u200broot",
		string([]byte{0xff}),
		strings.Repeat("x", runtimeidentity.MaximumExecutorIdentityBytes+1),
	}
	for _, value := range values {
		_, runtimeErr := runtimeidentity.ParseMember(value)
		_, frameworkErr := agent.ParseProcessID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("member policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
		_, runtimeErr = runtimeidentity.ParseRequest(value)
		_, frameworkErr = agent.ParseWaitID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("request policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
		_, runtimeErr = runtimeidentity.ParseEffect(value)
		_, frameworkErr = agent.ParseEffectID(value)
		if (runtimeErr == nil) != (frameworkErr == nil) {
			t.Errorf("effect policy differs for %q: Runtime=%v Agent Framework=%v", value, runtimeErr, frameworkErr)
		}
	}
}
