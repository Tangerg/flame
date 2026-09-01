package workspace

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestCatalogEnforcesTrustProjection(t *testing.T) {
	valid := HookCatalog{ProjectRoot: "/repo", ProjectTrusted: true, Hooks: []LifecycleHook{{
		Event: protocol.HookEventPreToolUse, Matcher: "shell*", Command: "check", Scope: protocol.HookScopeProject, Source: "/repo/.flame/hooks.json", Active: true,
	}, {Event: protocol.HookEventStop, Inject: "done", Scope: protocol.HookScopeGlobal, Source: "/home/.flame/hooks.json", Active: true}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
	invalid := valid
	invalid.Hooks = append([]LifecycleHook(nil), valid.Hooks...)
	invalid.Hooks[0].Active = false
	if err := invalid.Validate(); err == nil {
		t.Fatal("accepted project active state that disagrees with trust")
	}
}

func TestLifecycleHookUsesRuntimeWireBounds(t *testing.T) {
	for _, hook := range []LifecycleHook{
		{Event: protocol.HookEvent("Unknown"), Command: "check", Scope: protocol.HookScopeGlobal, Source: "/home/.flame/hooks.json", Active: true},
		{Event: protocol.HookEventStop, Command: "check", TimeoutMillis: 1_000_000, Scope: protocol.HookScopeGlobal, Source: "/home/.flame/hooks.json", Active: true},
	} {
		if err := hook.Validate(); err == nil {
			t.Fatalf("accepted hook outside runtime wire bounds: %+v", hook)
		}
	}
}

func TestCatalogValidatesTrustAcknowledgement(t *testing.T) {
	catalog := HookCatalog{ProjectRoot: "/repo", ProjectTrusted: true}
	if err := catalog.ValidateTrustAcknowledgement("/repo", true); err != nil {
		t.Fatalf("valid trust acknowledgement: %v", err)
	}
	if err := catalog.ValidateTrustAcknowledgement("/other", true); err == nil {
		t.Fatal("accepted trust acknowledgement for another project")
	}
	if err := catalog.ValidateTrustAcknowledgement("/repo", false); err == nil {
		t.Fatal("accepted the opposite trust acknowledgement")
	}
}
