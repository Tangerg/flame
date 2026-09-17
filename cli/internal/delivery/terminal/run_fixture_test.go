package terminal

import "github.com/Tangerg/flame/cli/internal/domain/agent"

func testStartRun(sessionID, text string) agent.StartRun {
	return agent.StartRun{
		SessionID: sessionID, Message: agent.Message{Text: text},
		Options: agent.RunOptions{},
	}
}
