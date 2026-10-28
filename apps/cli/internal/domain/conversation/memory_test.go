package conversation

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
)

const testMemoryID = "mem_00000000000000000000000000000001"

func TestTargetOwnsScopeWorkspaceInvariant(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     memory.UpdateV2OutputScope
		workspace string
		wantErr   bool
	}{
		{name: "project", scope: memory.UpdateV2OutputScopeProject, workspace: "/repo"},
		{name: "project on another operating system", scope: memory.UpdateV2OutputScopeProject, workspace: `C:\repo`},
		{name: "project without workspace", scope: memory.UpdateV2OutputScopeProject, wantErr: true},
		{name: "user", scope: memory.UpdateV2OutputScopeUser},
		{name: "user with workspace", scope: memory.UpdateV2OutputScopeUser, workspace: "/repo", wantErr: true},
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

func TestMemoryItemRejectsReversedTimestamps(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := memory.Item{Revision: 1, ID: testMemoryID, Scope: memory.UpdateV2OutputScopeProject, Content: "fact", Origin: memory.UpdateV2OutputOriginAuto, Status: memory.UpdateV2OutputStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.UpdatedAt = now.Add(-time.Second)
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "before creation") {
		t.Fatalf("ValidateMemoryItem() = %v", err)
	}
}

func TestMemoryProjectionRequiresVisiblePublisherState(t *testing.T) {
	now := time.Now()
	valid := memory.Item{Revision: 1, ID: testMemoryID, Scope: memory.UpdateV2OutputScopeUser, Content: "fact", Origin: memory.UpdateV2OutputOriginUser, Status: memory.UpdateV2OutputStatusActive, CreatedAt: now, UpdatedAt: now}
	for _, mutate := range []func(*memory.Item){
		func(row *memory.Item) { row.ID = "" },
		func(row *memory.Item) { row.Content = " " },
		func(row *memory.Item) { row.Scope = "session" },
		func(row *memory.Item) { row.Origin = "external" },
		func(row *memory.Item) { row.Status = "rejected" },
		func(row *memory.Item) { row.Status = memory.UpdateV2OutputStatusPending },
		func(row *memory.Item) { row.CreatedAt = time.Time{} },
		func(row *memory.Item) { row.UpdatedAt = time.Time{} },
	} {
		row := valid
		mutate(&row)
		if err := row.Validate(); err == nil {
			t.Fatalf("invalid visible projection accepted: %+v", row)
		}
	}
	valid.Origin, valid.Status = memory.UpdateV2OutputOriginAuto, memory.UpdateV2OutputStatusPending
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
}
