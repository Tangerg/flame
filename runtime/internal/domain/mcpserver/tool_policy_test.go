package mcpserver

import "testing"

func TestToolPolicy(t *testing.T) {
	tests := []struct {
		name    string
		servers []Server
		checks  map[ToolRef]struct {
			disabled     bool
			autoApproved bool
		}
	}{
		{
			name: "enabled servers contribute qualified tools",
			servers: []Server{
				{Name: testMCPServerName("files"), Enabled: true, ToolPolicy: testServerToolPolicy([]string{"write"}, []string{"read"})},
				{Name: testMCPServerName("db"), Enabled: true, ToolPolicy: testServerToolPolicy([]string{"drop"}, []string{"select"})},
			},
			checks: map[ToolRef]struct {
				disabled     bool
				autoApproved bool
			}{
				{Server: testMCPServerName("files"), Tool: testRemoteToolName("write")}: {disabled: true},
				{Server: testMCPServerName("files"), Tool: testRemoteToolName("read")}:  {autoApproved: true},
				{Server: testMCPServerName("db"), Tool: testRemoteToolName("drop")}:     {disabled: true},
				{Server: testMCPServerName("db"), Tool: testRemoteToolName("select")}:   {autoApproved: true},
				{Tool: testRemoteToolName("write")}:                                     {disabled: true},
			},
		},
		{
			name: "disabled servers contribute nothing",
			servers: []Server{
				{Name: testMCPServerName("files"), Enabled: false, ToolPolicy: testServerToolPolicy([]string{"write"}, []string{"read"})},
			},
			checks: map[ToolRef]struct {
				disabled     bool
				autoApproved bool
			}{
				{Server: testMCPServerName("files"), Tool: testRemoteToolName("write")}: {disabled: true},
				{Server: testMCPServerName("files"), Tool: testRemoteToolName("read")}:  {disabled: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewToolPolicy(tt.servers)
			for ref, want := range tt.checks {
				if got := policy.Disabled(ref); got != want.disabled {
					t.Errorf("Disabled(%+v) = %t, want %t", ref, got, want.disabled)
				}
				if got := policy.AutoApproved(ref); got != want.autoApproved {
					t.Errorf("AutoApproved(%+v) = %t, want %t", ref, got, want.autoApproved)
				}
			}
		})
	}
}

func TestZeroToolPolicyDisablesUnregisteredTools(t *testing.T) {
	var policy ToolPolicy
	ref := ToolRef{Server: testMCPServerName("server"), Tool: testRemoteToolName("tool")}
	if !policy.Disabled(ref) || policy.AutoApproved(ref) {
		t.Fatal("zero policy must disable unregistered tools without auto-approving them")
	}
}
