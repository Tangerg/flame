package terminal

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

func catalogPageSize(t testing.TB, rows int) conversation.PageSize {
	t.Helper()
	pageSize, err := conversation.NewPageSize(rows)
	if err != nil {
		t.Fatalf("conversation.NewPageSize(%d): %v", rows, err)
	}
	return pageSize
}
