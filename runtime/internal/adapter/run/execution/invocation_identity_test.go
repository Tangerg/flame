package execution

import (
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func TestModelInvocationIdentityBoundsMaximumFrameworkEffect(t *testing.T) {
	process, err := scoperun.ParseProcessID(strings.Repeat("x", runs.MaximumIdentityBytes) + "/" + strings.Repeat("y", runs.MaximumIdentityBytes))
	if err != nil {
		t.Fatalf("parse maximum Framework EffectID: %v", err)
	}
	frameworkID := scoperun.EffectID{Process: process, Step: math.MaxUint64, Index: math.MaxUint32}
	identity, err := modelInvocationIDFrom(frameworkID, math.MaxUint64)
	if err != nil {
		t.Fatalf("modelInvocationIDFrom maximum inputs: %v", err)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("generated invocation identity: %v", err)
	}
	if len(identity.String()) > runs.MaximumIdentityBytes {
		t.Fatalf("generated invocation identity has %d bytes, maximum is %d", len(identity.String()), runs.MaximumIdentityBytes)
	}
	repeated, err := modelInvocationIDFrom(frameworkID, math.MaxUint64)
	if err != nil || repeated != identity {
		t.Fatalf("repeated identity = %q, %v; want %q", repeated.String(), err, identity.String())
	}
	different, err := modelInvocationIDFrom(frameworkID, math.MaxUint64-1)
	if err != nil || different == identity {
		t.Fatalf("different sequence identity = %q, %v; want distinct", different.String(), err)
	}
}
