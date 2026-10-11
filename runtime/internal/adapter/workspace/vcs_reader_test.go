package workspace

import (
	"context"
	"errors"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

func TestVCSFailureProjectionRetainsCategoryAndCause(t *testing.T) {
	cause := errors.New("observation retired")
	for _, test := range []struct {
		mechanism error
		category  error
	}{
		{git.ErrUnavailable, workspaceapp.ErrVCSUnavailable},
		{git.ErrNotRepo, workspaceapp.ErrVCSUnavailable},
		{git.ErrNoBase, workspaceapp.ErrVCSBaseUnknown},
		{git.ErrResultTooLarge, workspaceapp.ErrVCSResultTooLarge},
	} {
		err := vcsError(errors.Join(test.mechanism, context.Canceled, cause))
		for _, want := range []error{test.mechanism, test.category, context.Canceled, cause} {
			if !errors.Is(err, want) {
				t.Fatalf("VCS failure projection lost %v: %v", want, err)
			}
		}
	}
}
