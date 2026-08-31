package mcpserver

import "testing"

func TestToolName(t *testing.T) {
	tests := []struct {
		name   string
		server string
		tool   string
		want   string
	}{
		{name: "prefixes server", server: "github", tool: "read", want: "github_read"},
		{name: "sanitizes unsupported bytes", server: "html.to.design", tool: "import-url", want: "html_to_design_import-url"},
		{name: "caps at provider limit", server: "srv", tool: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijkl", want: "srv_abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefgh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToolName(testMCPServerName(tc.server), testRemoteToolName(tc.tool)); got != tc.want {
				t.Fatalf("ToolName(%q, %q) = %q, want %q", tc.server, tc.tool, got, tc.want)
			}
		})
	}
}

func TestToolRefPreservesIdentityAcrossModelNameCollision(t *testing.T) {
	first := ToolRef{Server: testMCPServerName("a_b"), Tool: testRemoteToolName("c")}
	second := ToolRef{Server: testMCPServerName("a"), Tool: testRemoteToolName("b_c")}
	if first == second {
		t.Fatal("distinct tool references compared equal")
	}
	firstName := ToolName(first.Server, first.Tool)
	secondName := ToolName(second.Server, second.Tool)
	if firstName != secondName {
		t.Fatalf("fixture model names do not collide: %q != %q", firstName, secondName)
	}
}
