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
		scope     memory.ItemSnapshotScope
		workspace string
		wantErr   bool
	}{
		{name: "project", scope: memory.ItemSnapshotScopeProject, workspace: "/repo"},
		{name: "project on another operating system", scope: memory.ItemSnapshotScopeProject, workspace: `C:\repo`},
		{name: "project without workspace", scope: memory.ItemSnapshotScopeProject, wantErr: true},
		{name: "user", scope: memory.ItemSnapshotScopeUser},
		{name: "user with workspace", scope: memory.ItemSnapshotScopeUser, workspace: "/repo", wantErr: true},
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
	valid := memory.Item{Revision: 1, ID: testMemoryID, Scope: memory.ItemSnapshotScopeProject, Content: "fact", Origin: memory.ItemSnapshotOriginAuto, Status: memory.ItemSnapshotStatusPending, CreatedAt: now, UpdatedAt: now}
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
	valid := memory.Item{Revision: 1, ID: testMemoryID, Scope: memory.ItemSnapshotScopeUser, Content: "fact", Origin: memory.ItemSnapshotOriginUser, Status: memory.ItemSnapshotStatusActive, CreatedAt: now, UpdatedAt: now}
	for _, mutate := range []func(*memory.Item){
		func(row *memory.Item) { row.ID = "" },
		func(row *memory.Item) { row.Content = " " },
		func(row *memory.Item) { row.Scope = "session" },
		func(row *memory.Item) { row.Origin = "external" },
		func(row *memory.Item) { row.Status = "rejected" },
		func(row *memory.Item) { row.Status = memory.ItemSnapshotStatusPending },
		func(row *memory.Item) { row.CreatedAt = time.Time{} },
		func(row *memory.Item) { row.UpdatedAt = time.Time{} },
	} {
		row := valid
		mutate(&row)
		if err := row.Validate(); err == nil {
			t.Fatalf("invalid visible projection accepted: %+v", row)
		}
	}
	valid.Origin, valid.Status = memory.ItemSnapshotOriginAuto, memory.ItemSnapshotStatusPending
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
}
