package maintenance

import (
	"context"
	"strings"

	"github.com/Tangerg/scope/core/chat"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
)

const compactionPrompt = `You are compacting the earlier portion of a long agent
conversation into a faithful, STRUCTURED summary the agent will read as part of
its system prompt to continue WITHOUT losing key context. Be specific; quote
literal identifiers (file paths, function / type names, commands) so they stay
greppable. Treat every user request to remember, preserve, retain, or recall a
fact later as a hard retention requirement: record the exact literal value and
what it denotes. Resolve later references such as "the original marker" back to
that value; never substitute a later acknowledgement or paraphrase.

Output markdown under EXACTLY these headings (drop a heading only if truly empty):

## Goal
The user's original objective(s), in their own framing — quote the key request.

## Progress
What has been done so far: completed steps, what worked.

## Current state
Files / paths created or modified (with their paths) + each one's role; key
identifiers (functions, types, symbols) in play; command results worth keeping.

## Decisions & constraints
Choices made and WHY; user preferences / constraints stated (style, libraries,
dos & don'ts); exact facts explicitly reserved for later recall; approaches
rejected and the reason (so they aren't retried).

## Next steps
Remaining work + open questions — concrete and ordered.

Do NOT echo this instruction or restate the raw transcript; the agent receives
your sections verbatim.`

const (
	compactionSummaryOutputTokens int64 = 4096
	compactionModelPrefix               = "[Earlier conversation summary]\n"
)

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *Compactor) summarize(ctx context.Context, msgs []chat.Message) (string, error) {
	transcript := renderTranscript(msgs)

	text, err := c.client.Complete(ctx, modeladapter.AuxiliaryPrompt{
		SystemPrompt: compactionPrompt, UserPrompt: transcript,
		MaxInputBytes: maintenanceModelInputBytes, MaxOutputTokens: compactionSummaryOutputTokens,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
