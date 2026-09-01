package agent

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
		{name: "project with relative workspace", scope: protocol.AgentMemoryScopeProject, workspace: "repo", wantErr: true},
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

func TestItemRejectsBrokenReviewProjection(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := MemoryItem{ID: testMemoryID, Scope: protocol.AgentMemoryScopeProject, Content: "fact", Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Origin = protocol.AgentMemoryOriginUser
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestPatchRequiresAnIntentionalChange(t *testing.T) {
	t.Parallel()
	if err := (MemoryPatch{ID: testMemoryID}).Validate(); err == nil {
		t.Fatal("empty patch was accepted")
	}
	empty := "  "
	if err := (MemoryPatch{ID: testMemoryID, Content: &empty}).Validate(); err == nil {
		t.Fatal("blank content was accepted")
	}
	pinned := true
	if err := (MemoryPatch{ID: testMemoryID, Pinned: &pinned}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentMemoryMutationResultsMustFulfillTheCommand(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := MemoryItem{
		ID: testMemoryID, Scope: protocol.AgentMemoryScopeUser, Content: "authored", Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	target, err := NewMemoryTarget(protocol.AgentMemoryScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.ValidateAddResult(" authored ", valid); err != nil {
		t.Fatalf("valid add result: %v", err)
	}
	wrongAdd := valid
	wrongAdd.Content = "ignored"
	if err := target.ValidateAddResult("authored", wrongAdd); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("add result error = %v", err)
	}

	content, pinned := "edited", true
	patch := MemoryPatch{ID: valid.ID, Content: &content, Pinned: &pinned}
	updated := valid
	updated.Content, updated.Pinned = content, pinned
	if err := patch.ValidateResult(updated); err != nil {
		t.Fatalf("valid update result: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*MemoryItem)
		want   string
	}{
		{name: "identity", mutate: func(result *MemoryItem) { result.ID = "mem_00000000000000000000000000000002" }, want: "item"},
		{name: "content", mutate: func(result *MemoryItem) { result.Content = "ignored" }, want: "content"},
		{name: "pinned", mutate: func(result *MemoryItem) { result.Pinned = false }, want: "pinned"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := updated
			test.mutate(&result)
			err := patch.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}
