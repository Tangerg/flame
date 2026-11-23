package conversation

import (
	"errors"
	"fmt"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/core/chat"
)

var (
	ErrInvalid  = errors.New("conversation: invalid")
	ErrNotEmpty = errors.New("conversation: seed requires an empty conversation")
)

type Conversation struct {
	messages []chat.Message
}

func ValidateMessages(messages []chat.Message) error {
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("%w: message[%d]: %w", ErrInvalid, index, err)
		}
	}
	return nil
}

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

func (c Conversation) Messages() []chat.Message {
	out := make([]chat.Message, len(c.messages))
	for index, message := range c.messages {
		out[index] = message.Clone()
	}
	return out
}

func (c Conversation) Count() int { return len(c.messages) }

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

func (c Conversation) HasOpenToolCalls() bool { return !indexOpenToolCalls(c.messages).empty() }

func (c Conversation) CloseOpenToolCalls(result string) (Conversation, []chat.Message, error) {
	return c.CloseOpenToolCallsWithResults(result, nil)
}

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

type openToolCalls struct {
	ordered []chat.ToolCall
	current map[string]int
}

func indexOpenToolCalls(messages []chat.Message) *openToolCalls {
	calls := &openToolCalls{current: make(map[string]int)}
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
	calls.current[call.ID] = len(calls.ordered)
	calls.ordered = append(calls.ordered, call)
}

func (calls *openToolCalls) empty() bool { return len(calls.current) == 0 }

func (calls *openToolCalls) close(
	fallback string,
	completed completedToolResults,
) ([]chat.ToolResult, error) {
	results := make([]chat.ToolResult, 0, len(calls.current))
	for position, unresolved := range calls.ordered {
		if opened, present := calls.current[unresolved.ID]; !present || opened != position {
			continue
		}
		result, err := completed.resolve(unresolved, fallback)
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
		Output: toolcontract.NewTextOutput(fallback), IsError: true,
	}, nil
}

func (results completedToolResults) first() (string, bool) {
	for id := range results {
		return id, true
	}
	return "", false
}
