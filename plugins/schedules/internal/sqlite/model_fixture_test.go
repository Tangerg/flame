package sqlite

import modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"

func defaultModelSelection() modelchoice.Selection {
	selection, _ := modelchoice.New("anthropic", "claude")
	return selection
}
