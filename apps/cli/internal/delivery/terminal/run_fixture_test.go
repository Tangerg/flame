package terminal

import "github.com/Tangerg/flame/apps/cli/internal/domain/authoring"

func testStartRun(sessionID, text string) authoring.StartRun {
	return authoring.StartRun{
		SessionID: sessionID, Message: authoring.Message{Text: text},
		Options: authoring.RunOptions{},
	}
}
