package delivery

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestRuntimeOutputFailureOwnsItsCategory(t *testing.T) {
	failure := runtimeProduced("an invalid result", protocol.NewFailure(protocol.ErrInvalidRequest, "nested diagnostic"))
	if failure.Problem().Type != protocol.ProblemInternalError {
		t.Fatalf("output defect became a caller error: %+v", failure.Problem())
	}
}
