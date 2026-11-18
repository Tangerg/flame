package execution

import (
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testDefaultSelection() modelchoice.Selection {
	return testsupport.MustModelSelection("test-provider", "test-model")
}
