package schedules

import (
	"testing"

	"github.com/Tangerg/flame/runtime/pagination"
)

func explicitPageLimit(t *testing.T, value int) pagination.RequestedLimit {
	t.Helper()
	limit, err := pagination.NewLimit(value)
	if err != nil {
		t.Fatalf("pagination.NewLimit(%d): %v", value, err)
	}
	return limit
}
