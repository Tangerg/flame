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
		scope     memory.Scope
		workspace string
		wantErr   bool
	}{
		{name: "project", scope: memory.ScopeProject, workspace: "/repo"},
		{name: "project on another operating system", scope: memory.ScopeProject, workspace: `C:\repo`},
		{name: "project without workspace", scope: memory.ScopeProject, wantErr: true},
		{name: "user", scope: memory.ScopeUser},
		{name: "user with workspace", scope: memory.ScopeUser, workspace: "/repo", wantErr: true},
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
	valid := memory.Item{ID: testMemoryID, Scope: memory.ScopeProject, Content: "fact", Origin: memory.OriginAuto, Status: memory.StatusPending, CreatedAt: now, UpdatedAt: now}
	if err := ValidateMemoryItem(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.UpdatedAt = now.Add(-time.Second)
	if err := ValidateMemoryItem(invalid); err == nil || !strings.Contains(err.Error(), "before creation") {
		t.Fatalf("ValidateMemoryItem() = %v", err)
	}
}

func TestMemoryProjectionRequiresVisiblePublisherState(t *testing.T) {
	now := time.Now()
	valid := memory.Item{ID: testMemoryID, Scope: memory.ScopeUser, Content: "fact", Origin: memory.OriginUser, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now}
	for _, mutate := range []func(*memory.Item){
		func(row *memory.Item) { row.ID = "" },
		func(row *memory.Item) { row.Content = " " },
		func(row *memory.Item) { row.Scope = "session" },
		func(row *memory.Item) { row.Origin = "external" },
		func(row *memory.Item) { row.Status = "rejected" },
		func(row *memory.Item) { row.Status = memory.StatusPending },
		func(row *memory.Item) { row.CreatedAt = time.Time{} },
		func(row *memory.Item) { row.UpdatedAt = time.Time{} },
	} {
		row := valid
		mutate(&row)
		if err := ValidateMemoryItem(row); err == nil {
			t.Fatalf("invalid visible projection accepted: %+v", row)
		}
	}
	valid.Origin, valid.Status = memory.OriginAuto, memory.StatusPending
	if err := ValidateMemoryItem(valid); err != nil {
		t.Fatal(err)
	}
}
