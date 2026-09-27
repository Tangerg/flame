package terminal

import (
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
)

func testStartRun(sessionID, text string) prompt.StartRun {
	return prompt.StartRun{
		SessionID: sessionID, Message: prompt.Message{Text: text},
		Options: prompt.RunOptions{},
	}
}
