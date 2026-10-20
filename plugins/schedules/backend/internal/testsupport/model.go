package testsupport

import modelchoice "github.com/Tangerg/flame/runtime/model"

func DefaultModelSelection() modelchoice.Selection {
	selection, _ := modelchoice.New("anthropic", "claude")
	return selection
}
