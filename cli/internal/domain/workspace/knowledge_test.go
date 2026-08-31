package workspace

import "testing"

func TestTargetDoesNotLeakWorkspaceAcrossScopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     KnowledgeScope
		workspace string
		wantErr   bool
	}{
		{name: "cwd", scope: KnowledgeWorkingDirectory, workspace: "/repo"},
		{name: "project root", scope: KnowledgeProjectRoot, workspace: "/repo"},
		{name: "home", scope: KnowledgeHome},
		{name: "cwd without workspace", scope: KnowledgeWorkingDirectory, wantErr: true},
		{name: "home with workspace", scope: KnowledgeHome, workspace: "/repo", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewKnowledgeTarget(test.scope, test.workspace)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewTarget() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}
