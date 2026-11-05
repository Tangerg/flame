package maintenance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chat"
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
	maxModelInputBytes                   = 512 * 1024
	compactionTranscriptChunkBytes       = 384 * 1024
	compactionSummaryOutputTokens  int64 = 4096
	compactionModelPrefix                = "[Earlier conversation summary]\n"
)

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *Compactor) summarize(ctx context.Context, msgs []chat.Message) (string, error) {
	transcript := modeladapter.RenderCompactionTranscript(msgs)
	for {
		chunks := compactionChunks(transcript, compactionTranscriptChunkBytes)
		if len(chunks) == 0 {
			return "", errors.New("maintenance: cannot summarize an empty transcript")
		}
		var summaries strings.Builder
		for index, chunk := range chunks {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			input := chunk
			if len(chunks) > 1 {
				input = fmt.Sprintf("[Consecutive transcript portion %d of %d; a message may continue across portions. Preserve exact facts from this portion for the combined summary.]\n", index+1, len(chunks)) + chunk
			}
			text, err := c.client.Complete(ctx, modeladapter.AuxiliaryPrompt{
				Operation:    "compaction",
				SystemPrompt: compactionPrompt, UserPrompt: input,
				MaxInputBytes: maxModelInputBytes, MaxOutputTokens: compactionSummaryOutputTokens,
			})
			if err != nil {
				return "", err
			}
			if len(chunks) == 1 {
				return strings.TrimSpace(text), nil
			}
			fmt.Fprintf(&summaries, "[Summary of consecutive portion %d of %d]\n%s\n", index+1, len(chunks), strings.TrimSpace(text))
		}
		// A failed reduction cannot authorize silently deleting input or looping
		// forever. The caller retains its original history until final admission.
		if summaries.Len() >= len(transcript) {
			return "", errors.New("maintenance: partial summaries did not reduce the transcript")
		}
		transcript = summaries.String()
	}
}

// compactionChunks partitions every input byte exactly once. Prefer a message
// or line boundary in the latter half of each chunk, and never split a UTF-8 rune.
func compactionChunks(text string, limit int) []string {
	var chunks []string
	for len(text) > limit {
		end := limit
		if newline := strings.LastIndexByte(text[limit/2:limit], '\n'); newline >= 0 {
			end = limit/2 + newline + 1
		}
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		chunks = append(chunks, text[:end])
		text = text[end:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
