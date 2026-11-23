package execution

import (
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testDefaultSelection() flamemodel.Selection {
	return testsupport.MustModelSelection("test-provider", "test-model")
}
