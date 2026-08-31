package runtimebinding

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func catalogPageSize(t testing.TB, rows int) agent.PageSize {
	t.Helper()
	pageSize, err := agent.NewPageSize(rows)
	if err != nil {
		t.Fatalf("agent.NewPageSize(%d): %v", rows, err)
	}
	return pageSize
}
