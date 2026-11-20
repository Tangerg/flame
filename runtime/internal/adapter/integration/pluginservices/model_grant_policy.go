package pluginservices

import (
	"reflect"
	"sync"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
)

// auxiliaryTextRequest bounds the only model call a plugin callback may ask the
// Runtime to make on its behalf: one system and one user text message, an
// output cap and text output. Anything else could select tools, a model or
// provider options the callback was never granted.
type auxiliaryTextRequest struct {
	maxOutputTokens int64
	maxInputBytes   int
}

func (a auxiliaryTextRequest) admits(request chat.Request) bool {
	options := request.Options
	if options.MaxOutputTokens == nil || *options.MaxOutputTokens <= 0 || *options.MaxOutputTokens > a.maxOutputTokens {
		return false
	}
	if options.OutputFormat == nil || options.OutputFormat.Type != chat.OutputFormatText {
		return false
	}
	options.MaxOutputTokens, options.OutputFormat = nil, nil
	if !reflect.DeepEqual(options, chat.Options{}) || len(request.Tools) != 0 || request.ToolChoice != nil {
		return false
	}
	if len(request.Messages) != 2 || request.Messages[0].Role != chat.RoleSystem || request.Messages[1].Role != chat.RoleUser {
		return false
	}
	for _, message := range request.Messages {
		if !reflect.DeepEqual(message, chat.Message{Role: message.Role, Parts: []chat.Part{chat.NewTextPart(message.Text())}}) {
			return false
		}
	}
	return len(request.Messages[0].Text())+len(request.Messages[1].Text()) <= a.maxInputBytes
}

// externalCallLedger fences each callback-requested external call before its
// first I/O. Providers issue no receipt, so an identity is admitted once: a
// repeat is refused rather than replayed, and the budget counts admissions.
type externalCallLedger struct {
	owner string
	limit int
	mu    sync.Mutex
	calls map[string]externalCall
}

type externalCall struct {
	method string
	input  framevalue.JSON
}

const (
	externalModelCall     = "model"
	externalEmbeddingCall = "embedding"
)

const (
	refusalModelEnvelope         = "model_envelope"
	refusalEmbeddingEnvelope     = "embedding_envelope"
	refusalModelRedelivery       = "model_redelivery"
	refusalModelBudget           = "model_budget"
	refusalModelRoleUnconfigured = "model_role_unconfigured"
	refusalObserverContract      = "observer_contract"
	refusalReadBudget            = "read_budget"
)

func newExternalCallLedger(owner string, limit int) *externalCallLedger {
	return &externalCallLedger{owner: owner, limit: limit, calls: make(map[string]externalCall)}
}

func (l *externalCallLedger) admit(id, method string, input framevalue.JSON) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	call := externalCall{method: method, input: input}
	if original, found := l.calls[id]; found {
		if original != call {
			return &framecall.Refusal{Code: framecall.ConflictCode, Message: framecall.ErrConflict.Error()}
		}
		return &framecall.Refusal{Code: refusalModelRedelivery, Message: l.owner + ": original model invocation cannot be redelivered"}
	}
	if len(l.calls) >= l.limit {
		return &framecall.Refusal{Code: refusalModelBudget, Message: l.owner + ": model budget exhausted"}
	}
	l.calls[id] = call
	return nil
}
