package plugin

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/scope/core/chat"
)

// ModelContextRequest describes the captured model and current request before
// context capacity admission. It does not authorize model execution or routing.
type ModelContextRequest struct {
	Provider string       `json:"provider"`
	Model    string       `json:"model"`
	Request  chat.Request `json:"request"`
	// Host-authored user-role trailers cannot identify the original caller input.
	UserMessage *chat.Message `json:"userMessage,omitzero"`
}

// ModelContextResult supplies additional instructions/retrieval material. The
// consumer preserves authored conversation and Tool contracts under their owners.
type ModelContextResult struct {
	Messages []chat.Message `json:"messages"`
}

func ModelContextMethod() (*Method[ModelContextRequest, ModelContextResult], error) {
	return NewMethod[ModelContextRequest, ModelContextResult](MethodConfig{Name: "flame.model-context/contribute.v2", Kind: capability.Query, Target: "call", Description: "Contribute additional system context using the captured request and its original user input before capacity admission."})
}

// ModelContextContract orders independent context contributions before the final
// request is admitted. It grants no next proxy, model call or route replacement.
func ModelContextContract() (*capability.Contract, error) { return modelContextContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelContextContract = sync.OnceValues(func() (*capability.Contract, error) {
	method, err := ModelContextMethod()
	if err != nil {
		return nil, err
	}
	return capability.New(capability.Config{Reference: "flame.model-context.v2", Composition: capability.Decoration, Scope: "call"}, method.Descriptor())
})

func (r ModelContextResult) Validate() error {
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.Role != chat.RoleSystem {
			return errors.New("model context: contributions must contain system messages")
		}
	}
	return nil
}

func (r ModelContextRequest) Validate() error {
	if r.Provider == "" || r.Model == "" {
		return errors.New("model context: captured provider and model are required")
	}
	if r.UserMessage != nil {
		if err := r.UserMessage.Validate(); err != nil {
			return err
		}
		if r.UserMessage.Role != chat.RoleUser {
			return errors.New("model context: original user input must have the user role")
		}
	}
	return r.Request.Validate()
}
