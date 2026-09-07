package maintenance

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

const (
	// maintenanceModelInputBytes is the hard system+user request envelope for
	// maintenance calls. Transcript rendering uses the smaller allocation below
	// so fixed instructions and capability-specific framing still fit.
	maintenanceModelInputBytes = 512 * 1024
	// maintenanceTranscriptBytes leaves room in the request envelope for
	// fixed instructions and capability-specific framing.
	maintenanceTranscriptBytes = 384 * 1024
	// maintenanceMessageBytes prevents one message from consuming the whole
	// transcript when only a few messages are present.
	maintenanceMessageBytes = 24 * 1024
)

// renderTranscript flattens messages into a plain-text role-tagged transcript
// a summariser or consolidator can read. It is lossy by design: the auxiliary
// model needs the conversation's gist, not a second lossless journal.
//
// Every call is bounded as a whole and gives each message an equal allocation,
// capped by maintenanceMessageBytes. There is intentionally no uncapped mode.
func renderTranscript(msgs []chat.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	framingBytes := len(msgs) // one trailing newline per message
	messageBudget := max(1, (maintenanceTranscriptBytes-framingBytes)/len(msgs))
	messageBudget = min(messageBudget, maintenanceMessageBytes)

	var transcript strings.Builder
	for _, msg := range msgs {
		transcript.WriteString(renderTranscriptMessage(msg, messageBudget))
		transcript.WriteByte('\n')
	}
	return capText(transcript.String(), maintenanceTranscriptBytes)
}

func renderTranscriptMessage(msg chat.Message, budget int) string {
	switch msg.Role {
	case chat.RoleSystem:
		return renderTranscriptParts("[system] ", transcriptTextValues(msg), "", budget)
	case chat.RoleUser:
		return renderTranscriptParts("[user] ", transcriptTextValues(msg), "", budget)
	case chat.RoleAssistant:
		return renderTranscriptParts("[assistant] ", transcriptTextValues(msg), "", budget)
	case chat.RoleTool:
		results := make([]string, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			if part.Kind == chat.PartToolResult && part.ToolResult != nil {
				results = append(results, renderToolOutput(part.ToolResult.Output))
			}
		}
		return renderTranscriptParts("[tool] ", results, " ", budget)
	default:
		return capText(fmt.Sprintf("[%s] (unrecognized)", msg.Role), budget)
	}
}

func transcriptTextValues(msg chat.Message) []string {
	values := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if isTranscriptText(part.Kind) {
			values = append(values, part.Text)
		}
	}
	return values
}

func renderTranscriptParts(prefix string, values []string, separator string, budget int) string {
	if len(values) == 0 {
		return capText(prefix, budget)
	}
	valueBudget := max(1, (budget-len(prefix)-len(separator)*len(values))/len(values))
	var rendered strings.Builder
	rendered.WriteString(prefix)
	for _, value := range values {
		rendered.WriteString(capText(value, valueBudget))
		rendered.WriteString(separator)
	}
	return capText(rendered.String(), budget)
}

func isTranscriptText(kind chat.PartKind) bool {
	return kind == chat.PartText || kind == chat.PartRefusal
}

func saturatedAdd(total int, values ...int) int {
	for _, value := range values {
		if value > math.MaxInt-total {
			return math.MaxInt
		}
		total += value
	}
	return total
}

// headTail reduces s to a head (¾) + tail (¼) preview when it exceeds limit,
// including the elision marker inside that exact byte limit. Cuts snap to rune
// boundaries so a multibyte rune is never split. Shared by transient model
// rendering and the compaction ladder's stored trim.
func headTail(s string, limit int, marker func(elided int) string) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	keptBytes := limit
	for {
		head, tailStart := keptBytes*3/4, len(s)-keptBytes/4
		for head > 0 && !utf8.RuneStart(s[head]) {
			head--
		}
		for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
			tailStart++
		}
		elision := marker(tailStart - head)
		if head+len(elision)+len(s)-tailStart <= limit {
			return s[:head] + elision + s[tailStart:]
		}
		keptBytes = limit - len(elision)
		if keptBytes <= 0 {
			end := min(limit, len(elision))
			for end > 0 && end < len(elision) && !utf8.RuneStart(elision[end]) {
				end--
			}
			return elision[:end]
		}
	}
}

// capText bounds transient model input with a marked head+tail preview.
func capText(s string, limit int) string {
	return headTail(s, limit, func(elided int) string {
		return fmt.Sprintf("\n…[%d bytes elided for auxiliary model input]…\n", elided)
	})
}
