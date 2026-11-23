package model

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

const (
	mediaMarker                = "[media content omitted]"
	maintenanceTranscriptBytes = 384 * 1024
	maintenanceMessageBytes    = 24 * 1024
)

func RenderTranscript(msgs []chat.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	framingBytes := len(msgs)
	messageBudget := max(1, (maintenanceTranscriptBytes-framingBytes)/len(msgs))
	messageBudget = min(messageBudget, maintenanceMessageBytes)

	var transcript strings.Builder
	for _, msg := range msgs {
		transcript.WriteString(renderTranscriptMessage(msg, messageBudget))
		transcript.WriteByte('\n')
	}
	return capText(transcript.String(), maintenanceTranscriptBytes)
}

func RenderCompactionTranscript(msgs []chat.Message) string {
	var transcript strings.Builder
	for _, message := range msgs {
		fmt.Fprintf(&transcript, "[%s] ", message.Role)
		if message.Role == chat.RoleTool {
			for _, part := range message.Parts {
				if result := part.ToolResult; result != nil {
					fmt.Fprintf(&transcript, "[result id=%q name=%q error=%t] %s\n", result.ID, result.Name, result.IsError, renderToolOutput(result.Output))
				}
			}
		} else {
			for _, value := range transcriptValues(message) {
				transcript.WriteString(value)
				transcript.WriteByte('\n')
			}
		}
		transcript.WriteByte('\n')
	}
	return transcript.String()
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

// Identity and error status are budgeted apart from the body: budgeting them
// together elides error=true before the output it describes.
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
	return capText(rendered.String(), budget)
}

func transcriptValues(msg chat.Message) []string {
	values := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch part.Kind {
		case chat.PartText, chat.PartRefusal:
			values = append(values, part.Text)
		case chat.PartMedia:
			values = append(values, mediaMarker)
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

const tailBudgetDivisor = 4

func HeadTailCut(s string, kept int) (head, tailStart int) {
	kept = min(max(kept, 0), len(s))
	head, tailStart = kept*(tailBudgetDivisor-1)/tailBudgetDivisor, len(s)-kept/tailBudgetDivisor
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	return head, max(head, tailStart)
}

func HeadTail(s string, limit int, marker func(elided int) string) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	keptBytes := limit
	for {
		head, tailStart := HeadTailCut(s, keptBytes)
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

func capText(s string, limit int) string {
	return HeadTail(s, limit, func(elided int) string {
		return fmt.Sprintf("\n…[%d bytes elided for auxiliary model input]…\n", elided)
	})
}
