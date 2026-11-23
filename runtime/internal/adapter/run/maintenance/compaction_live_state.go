package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/scope/core/chat"
)

type RetainedShell struct {
	ID      string
	Command string
}

type LiveStateSnapshot struct {
	Shells []RetainedShell
}

func (l LiveStateSnapshot) empty() bool {
	return len(l.Shells) == 0
}

type LiveStateSnapshotter func(ctx context.Context, sessionID identity.SessionID) LiveStateSnapshot

func liveStateReminder(snap LiveStateSnapshot) (chat.Message, bool) {
	if snap.empty() {
		return chat.Message{}, false
	}
	var b strings.Builder
	b.WriteString("The earlier conversation was summarized to save context. These shell handles were still retained at compaction time. Commands may be running or finished with unread output:\n")
	if len(snap.Shells) > 0 {
		b.WriteString("\nBackground shells (read their output with read_shell_output, stop them with stop_shell):")
		for _, sh := range snap.Shells {
			fmt.Fprintf(&b, "\n  - %s: %s", sh.ID, sh.Command)
		}
		b.WriteByte('\n')
	}
	return chat.NewSystemMessage(executionadapter.FrameRuntimeContext(executionadapter.RuntimeContextRetainedShells, strings.TrimSuffix(b.String(), "\n"))), true
}
