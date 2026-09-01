package workspace

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestTargetDoesNotLeakWorkspaceAcrossScopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     protocol.KnowledgeScope
		workspace string
		wantErr   bool
	}{
		{name: "cwd", scope: protocol.KnowledgeScopeCWD, workspace: "/repo"},
		{name: "project root", scope: protocol.KnowledgeScopeProjectRoot, workspace: "/repo"},
		{name: "home", scope: protocol.KnowledgeScopeHome},
		{name: "cwd without workspace", scope: protocol.KnowledgeScopeCWD, wantErr: true},
		{name: "home with workspace", scope: protocol.KnowledgeScopeHome, workspace: "/repo", wantErr: true},
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
