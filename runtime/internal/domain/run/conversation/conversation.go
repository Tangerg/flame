// Package conversation owns the model-context message sequence independently
// from Runs, transcript observations, and executor working state.
package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

var (
	// ErrInvalid reports a malformed conversation value.
	ErrInvalid = errors.New("conversation: invalid")
	// ErrNotEmpty reports an attempt to seed a conversation that already has
	// messages. Seed is a fork/import operation, never append under another name.
	ErrNotEmpty = errors.New("conversation: seed requires an empty conversation")
)

// Conversation is an ownership-isolated model-context sequence. It contains no
// persistence, Run, transcript, or executor state.
type Conversation struct {
	messages []chat.Message
}

// ValidateMessages borrows a sequence while checking the model-context contract.
// Persistence and synchronous use cases can validate without changing representation.
func ValidateMessages(messages []chat.Message) error {
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("%w: message[%d]: %w", ErrInvalid, index, err)
		}
		if err := ValidateMessageIdentities(message); err != nil {
			return fmt.Errorf("%w: message[%d]: %w", ErrInvalid, index, err)
		}
	}
	return nil
}

// New validates and copies borrowed messages into an immutable conversation.
func New(messages []chat.Message) (Conversation, error) {
	if err := ValidateMessages(messages); err != nil {
		return Conversation{}, err
	}
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		owned[index] = message.Clone()
	}
	return Conversation{messages: owned}, nil
}

// Messages returns an ownership-isolated snapshot in model-context order.
func (c Conversation) Messages() []chat.Message {
	out := make([]chat.Message, len(c.messages))
	for index, message := range c.messages {
		out[index] = message.Clone()
	}
	return out
}

// Count returns the current message watermark.
func (c Conversation) Count() int { return len(c.messages) }

// Append returns the conversation extended by messages in model-context order.
func (c Conversation) Append(messages ...chat.Message) (Conversation, error) {
	addition, err := New(messages)
	if err != nil {
		return Conversation{}, err
	}
	combined := make([]chat.Message, 0, len(c.messages)+len(addition.messages))
	combined = append(combined, c.messages...)
	combined = append(combined, addition.messages...)
	return Conversation{messages: combined}, nil
}

// CloseOpenToolCalls returns the conversation with one error result appended
// for every provider ToolCall that has no later ToolResult. The results are
// ordered by the calls' first unresolved occurrence and share one Tool message,
// matching the provider-neutral conversation protocol. A resolved call ID may
// be reused by a later generation without reopening the former generation.
func (c Conversation) CloseOpenToolCalls(result string) (Conversation, []chat.Message, error) {
	return c.CloseOpenToolCallsWithResults(result, nil)
}

// CloseOpenToolCallsWithResults is CloseOpenToolCalls with authoritative
// results for calls that completed out of order but could not yet be appended.
// Every supplied result must match the latest unresolved generation. The final
// Tool message follows provider call order and fills only the remaining calls
// with the fallback error, so a terminal boundary preserves known output
// without leaving a malformed model history.
func (c Conversation) CloseOpenToolCallsWithResults(
	result string,
	completed []chat.ToolResult,
) (Conversation, []chat.Message, error) {
	openCalls := indexOpenToolCalls(c.messages)
	knownResults, err := newCompletedToolResults(completed)
	if err != nil {
		return Conversation{}, nil, err
	}
	if openCalls.empty() && knownResults.empty() {
		return c, nil, nil
	}
	results, err := openCalls.close(result, knownResults)
	if err != nil {
		return Conversation{}, nil, err
	}
	if len(results) == 0 {
		return c, nil, nil
	}
	appended := []chat.Message{chat.NewToolMessage(results...)}
	closed, err := c.Append(appended...)
	if err != nil {
		return Conversation{}, nil, err
	}
	return closed, appended, nil
}

type toolCallGeneration int

type openToolCall struct {
	call       chat.ToolCall
	generation toolCallGeneration
}

type openToolCalls struct {
	ordered    []openToolCall
	current    map[string]toolCallGeneration
	generation toolCallGeneration
}

func indexOpenToolCalls(messages []chat.Message) openToolCalls {
	calls := openToolCalls{current: make(map[string]toolCallGeneration)}
	for _, message := range messages {
		for _, part := range message.Parts {
			calls.observe(part)
		}
	}
	return calls
}

func (calls *openToolCalls) observe(part chat.Part) {
	if call := part.ToolCall; call != nil {
		calls.open(*call)
	}
	if result := part.ToolResult; result != nil {
		delete(calls.current, result.ID)
	}
}

func (calls *openToolCalls) open(call chat.ToolCall) {
	if _, alreadyOpen := calls.current[call.ID]; alreadyOpen {
		return
	}
	calls.generation++
	calls.current[call.ID] = calls.generation
	calls.ordered = append(calls.ordered, openToolCall{
		call:       call,
		generation: calls.generation,
	})
}

func (calls openToolCalls) empty() bool { return len(calls.current) == 0 }

func (calls openToolCalls) close(
	fallback string,
	completed completedToolResults,
) ([]chat.ToolResult, error) {
	results := make([]chat.ToolResult, 0, len(calls.current))
	for _, unresolved := range calls.ordered {
		if !calls.isCurrent(unresolved) {
			continue
		}
		result, err := completed.resolve(unresolved.call, fallback)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if id, unexpected := completed.first(); unexpected {
		return nil, fmt.Errorf(
			"%w: completed ToolResult %q has no unresolved ToolCall",
			ErrInvalid,
			id,
		)
	}
	return results, nil
}

func (calls openToolCalls) isCurrent(candidate openToolCall) bool {
	generation, present := calls.current[candidate.call.ID]
	return present && generation == candidate.generation
}

type completedToolResults map[string]chat.ToolResult

func newCompletedToolResults(results []chat.ToolResult) (completedToolResults, error) {
	completed := make(completedToolResults, len(results))
	for _, result := range results {
		if _, duplicate := completed[result.ID]; duplicate {
			return nil, fmt.Errorf(
				"%w: completed ToolResult %q is repeated",
				ErrInvalid,
				result.ID,
			)
		}
		completed[result.ID] = result
	}
	return completed, nil
}

func (results completedToolResults) empty() bool { return len(results) == 0 }

func (results completedToolResults) resolve(
	call chat.ToolCall,
	fallback string,
) (chat.ToolResult, error) {
	if result, found := results[call.ID]; found {
		if result.Name != call.Name {
			return chat.ToolResult{}, fmt.Errorf(
				"%w: completed ToolResult %q names %q, want %q",
				ErrInvalid,
				result.ID,
				result.Name,
				call.Name,
			)
		}
		delete(results, call.ID)
		return result, nil
	}
	return chat.ToolResult{
		ID: call.ID, Name: call.Name,
		Output: chat.NewTextToolOutput(fallback), IsError: true,
	}, nil
}

func (results completedToolResults) first() (string, bool) {
	for id := range results {
		return id, true
	}
	return "", false
}
