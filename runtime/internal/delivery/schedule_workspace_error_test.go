package delivery

import (
	"errors"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

// TestScheduleErrorsUseTheWorkspaceTranslation covers the ownership, not one
// error: a schedule carries a cwd, so any workspace failure can surface from
// these methods and all of them must reach the JSON-RPC vocabulary through the
// single translation rather than leaking as an internal error.
func TestScheduleErrorsUseTheWorkspaceTranslation(t *testing.T) {
	for _, test := range []struct {
		source error
		want   error
	}{
		{workspaceapp.ErrCWDUnavailable, protocol.ErrWorkspaceUnavailable},
		{workspaceapp.ErrPathOutsideRoot, protocol.ErrPathOutsideRoot},
		{workspaceapp.ErrVCSUnavailable, protocol.ErrVcsUnavailable},
		{workspaceapp.ErrPageCursor, protocol.ErrInvalidParams},
	} {
		if got := mapScheduleErr(test.source, "schedules.update", "sch_1"); !errors.Is(got, test.want) {
			t.Errorf("mapScheduleErr(%v) = %v, want %v", test.source, got, test.want)
		}
	}
}
