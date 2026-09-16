package maintenance

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

const (
	transcriptMediaMarker = "[media content omitted]"
	// maintenanceModelInputBytes is the hard system+user request envelope for
	// maintenance calls. Transcript rendering uses the smaller allocation below
	// so fixed instructions and capability-specific framing still fit.
	maintenanceModelInputBytes = 512 * 1024
	// maintenanceTranscriptBytes is the aggregate model-input allocation for
	// one rendered conversation. It mirrors the proven app2 envelope while
	// keeping the hard request cap in adapter/model as the final guard.
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
		return renderTranscriptParts("[system] ", transcriptValues(msg), budget)
	case chat.RoleUser:
		return renderTranscriptParts("[user] ", transcriptValues(msg), budget)
	case chat.RoleAssistant:
		return renderTranscriptParts("[assistant] ", transcriptValues(msg), budget)
	case chat.RoleTool:
		return renderToolResults(msg, budget)
	default:
		return capText(fmt.Sprintf("[%s] (unrecognized)", msg.Role), budget)
	}
}

// renderToolResults keeps each result's identity and error status out of the
// byte budget and spends what is left on output. Those are the facts a summary
// may not lose, and they sit in the middle of the line: budgeting them together
// with the body elides "error=true" before the output it describes, which is how
// a failed operation reaches the summariser looking like a successful one. A
// result whose header does not fit is counted, never half-written.
func renderToolResults(msg chat.Message, budget int) string {
	const prefix = "[tool] "
	var headers, outputs []string
	for _, part := range msg.Parts {
		if part.Kind != chat.PartToolResult || part.ToolResult == nil {
			continue
		}
		result := part.ToolResult
		headers = append(headers, fmt.Sprintf("[result id=%q name=%q error=%t] ",
			result.ID, result.Name, result.IsError))
		outputs = append(outputs, renderToolOutput(result.Output))
	}
	if len(headers) == 0 {
		return capText(prefix, budget)
	}

	// Reserved at its widest so admitting one more result can never make the
	// note that reports the rest no longer fit.
	note := fmt.Sprintf("(%d results elided)\n", len(headers))
	remaining, kept := budget-len(prefix), 0
	for kept < len(headers) {
		reserve := 0
		if kept+1 < len(headers) {
			reserve = len(note)
		}
		if len(headers[kept])+1+reserve > remaining {
			break
		}
		remaining -= len(headers[kept]) + 1
		kept++
	}

	var rendered strings.Builder
	rendered.WriteString(prefix)
	if kept < len(headers) {
		remaining -= len(note)
	}
	outputBudget := 0
	if kept > 0 {
		outputBudget = remaining / kept
	}
	for index := range kept {
		rendered.WriteString(headers[index])
		if outputBudget > 0 {
			rendered.WriteString(capText(outputs[index], outputBudget))
		}
		rendered.WriteByte('\n')
	}
	if kept < len(headers) {
		fmt.Fprintf(&rendered, "(%d results elided)\n", len(headers)-kept)
	}
	// Keeping a header whole is the rule inside a budget that can hold one. A
	// budget too small for even the first header keeps none, so the only thing
	// left to bound is the note — and every message owes the transcript its
	// equal share, or one of them silently spends a later message's.
	return capText(rendered.String(), budget)
}

func transcriptValues(msg chat.Message) []string {
	values := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch part.Kind {
		case chat.PartText, chat.PartRefusal:
			values = append(values, part.Text)
		case chat.PartMedia:
			values = append(values, transcriptMediaMarker)
		case chat.PartToolCall:
			if call := part.ToolCall; call != nil {
				values = append(values, fmt.Sprintf("[call id=%q name=%q] arguments=%s", call.ID, call.Name, call.Arguments))
			}
		}
	}
	return values
}

func renderTranscriptParts(prefix string, values []string, budget int) string {
	if len(values) == 0 {
		return capText(prefix, budget)
	}
	valueBudget := max(1, (budget-len(prefix)-len(values))/len(values))
	var rendered strings.Builder
	rendered.WriteString(prefix)
	for _, value := range values {
		rendered.WriteString(capText(value, valueBudget))
		rendered.WriteByte('\n')
	}
	return capText(rendered.String(), budget)
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
