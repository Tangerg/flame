package operation

import "testing"

func TestIdentityPreservesExactCommandScope(t *testing.T) {
	commands := []struct{ namespace, key string }{
		{"store-a", "command"},
		{"store-b", "command"},
		{"store-a", "\xff"},
		{"store-a", "\xfe"},
		{"ab", "c"},
		{"a", "bc"},
	}
	seen := make(map[string]bool)
	for _, command := range commands {
		id := Identity(Bind(t.Context(), command.namespace, command.key))
		if id == "" || seen[id] {
			t.Fatalf("command (%q, %q) lost its exact scope", command.namespace, command.key)
		}
		seen[id] = true
		if replay := Identity(Bind(t.Context(), command.namespace, command.key)); replay != id {
			t.Fatal("retry changed the command identity")
		}
	}
	first := Identity(Bind(t.Context(), "store-a", ""))
	second := Identity(Bind(t.Context(), "store-a", ""))
	if first == "" || second == "" || first == second {
		t.Fatal("unkeyed invocations reused a command identity")
	}
}
