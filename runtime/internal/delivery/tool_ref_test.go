package delivery

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestInvalidToolProjectionPreservesFailure(t *testing.T) {
	if _, err := presentToolRef(tool.Ref{}); !errors.Is(err, tool.ErrInvalidRef) {
		t.Fatalf("invalid tool projection = %v", err)
	}
}
