package agentexec

import (
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/executoridentity"
	agent "github.com/Tangerg/scope/agent"
)

func TestModelInvocationIdentityBoundsMaximumFrameworkEffect(t *testing.T) {
	frameworkID, err := agent.ParseEffectID(strings.Repeat("x", executoridentity.MaximumBytes))
	if err != nil {
		t.Fatalf("parse maximum Framework EffectID: %v", err)
	}
	identity, err := modelInvocationIDFrom(frameworkID, math.MaxUint32)
	if err != nil {
		t.Fatalf("modelInvocationIDFrom maximum inputs: %v", err)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("generated invocation identity: %v", err)
	}
	if len(identity.String()) > executoridentity.MaximumBytes {
		t.Fatalf("generated invocation identity has %d bytes, maximum is %d", len(identity.String()), executoridentity.MaximumBytes)
	}
	repeated, err := modelInvocationIDFrom(frameworkID, math.MaxUint32)
	if err != nil || repeated != identity {
		t.Fatalf("repeated identity = %q, %v; want %q", repeated.String(), err, identity.String())
	}
	different, err := modelInvocationIDFrom(frameworkID, math.MaxUint32-1)
	if err != nil || different == identity {
		t.Fatalf("different sequence identity = %q, %v; want distinct", different.String(), err)
	}
}
