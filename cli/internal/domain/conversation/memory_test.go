package conversation

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

const testMemoryID = "mem_00000000000000000000000000000001"

func TestTargetOwnsScopeWorkspaceInvariant(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     protocol.AgentMemoryScope
		workspace string
		wantErr   bool
	}{
		{name: "project", scope: protocol.AgentMemoryScopeProject, workspace: "/repo"},
		{name: "project on another operating system", scope: protocol.AgentMemoryScopeProject, workspace: `C:\repo`},
		{name: "project without workspace", scope: protocol.AgentMemoryScopeProject, wantErr: true},
		{name: "user", scope: protocol.AgentMemoryScopeUser},
		{name: "user with workspace", scope: protocol.AgentMemoryScopeUser, workspace: "/repo", wantErr: true},
		{name: "unknown", scope: "session", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewMemoryTarget(test.scope, test.workspace)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewMemoryTarget() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

// TestMemoryItemRejectsReversedTimestamps covers the one fact the Runtime wire
// contract cannot state about an item: two independent timestamps in the wrong
// order. Origin, status, scope and their agreement are the endpoint's answer.
func TestMemoryItemRejectsReversedTimestamps(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := protocol.AgentMemoryItem{ID: testMemoryID, Scope: protocol.AgentMemoryScopeProject, Content: "fact", Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := ValidateMemoryItem(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.UpdatedAt = now.Add(-time.Second)
	if err := ValidateMemoryItem(invalid); err == nil || !strings.Contains(err.Error(), "before creation") {
		t.Fatalf("ValidateMemoryItem() = %v", err)
	}
}
