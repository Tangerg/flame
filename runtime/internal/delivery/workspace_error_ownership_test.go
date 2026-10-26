package delivery

import (
	"errors"
	"testing"

	workspacepaths "github.com/Tangerg/flame/packages/go-sdk/workspace"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestHandlersUseTheOneWorkspaceTranslation(t *testing.T) {
	for _, test := range []struct {
		source error
		want   error
	}{
		{workspacepaths.ErrCWDUnavailable, protocol.ErrWorkspaceUnavailable},
		{workspaceapp.ErrPathOutsideRoot, protocol.ErrPathOutsideRoot},
		{workspaceapp.ErrVCSUnavailable, protocol.ErrVcsUnavailable},
		{workspaceapp.ErrPageCursor, protocol.ErrInvalidParams},
	} {
		if got := wireSessionErr(test.source); !errors.Is(got, test.want) {
			t.Errorf("wireSessionErr(%v) = %v, want %v", test.source, got, test.want)
		}
	}
}
