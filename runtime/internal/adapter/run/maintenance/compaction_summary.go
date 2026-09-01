package maintenance

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"

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

var errEmptyCompactionSummary = errors.New("compactor: summary is empty")

const (
	compactionSummaryOutputTokens int64 = 4096
	compactionModelPrefix               = "[Earlier conversation summary]\n"
)

// compactionSummary keeps the user-readable summary separate from the model
// framing used when the summary is written back into chat history. The raw text
// crosses the application boundary; the framed message remains an adapter
// concern.
type compactionSummary struct {
	text string
}

func newCompactionSummary(text string) (compactionSummary, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return compactionSummary{}, errEmptyCompactionSummary
	}
	return compactionSummary{text: text}, nil
}

func (s compactionSummary) Text() string {
	return s.text
}

func (s compactionSummary) Message() chat.Message {
	return chat.NewSystemMessage(compactionModelPrefix + s.text)
}

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *Compactor) summarize(ctx context.Context, msgs []chat.Message) (compactionSummary, error) {
	transcript := renderTranscript(msgs)

	var client *chatclient.Client
	if c.client != nil {
		client = c.client(ctx)
	}
	text, err := modeladapter.CompleteAuxiliary(ctx, client, modeladapter.AuxiliaryPrompt{
		SystemPrompt: compactionPrompt, UserPrompt: transcript,
		MaxInputBytes: maintenanceModelInputBytes, MaxOutputTokens: compactionSummaryOutputTokens,
	})
	if err != nil {
		return compactionSummary{}, err
	}
	return newCompactionSummary(text)
}
