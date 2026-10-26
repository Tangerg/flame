package testsupport

import modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"

func DefaultModelSelection() modelchoice.Selection {
	selection, _ := modelchoice.New("anthropic", "claude")
	return selection
}
