package agent

import "testing"

func TestCommandIDHasAStableValidatedWireIdentity(t *testing.T) {
	id := NewCommandID([CommandIDEntropyBytes]byte{})
	if got, want := string(id), "cli_00000000000000000000000000000000"; got != want {
		t.Fatalf("command id = %q, want %q", got, want)
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("generated command id is invalid: %v", err)
	}
	for _, invalid := range []CommandID{"", "other_00000000000000000000000000000000", "cli_short", "cli_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid command id %q was accepted", invalid)
		}
	}
}
