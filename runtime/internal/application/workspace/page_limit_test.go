package workspace

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
)

func explicitPageLimit(t *testing.T, value int) pagination.RequestedLimit {
	t.Helper()
	limit, err := pagination.NewLimit(value)
	if err != nil {
		t.Fatalf("pagination.NewLimit(%d): %v", value, err)
	}
	return limit
}
