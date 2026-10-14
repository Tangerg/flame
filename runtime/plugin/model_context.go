package plugin

import (
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/scope/core/chat"
)

// ModelContextRequest describes the captured model and current request before
// context capacity admission. It does not authorize model execution or routing.
type ModelContextRequest struct {
	Provider string       `json:"provider"`
	Model    string       `json:"model"`
	Request  chat.Request `json:"request"`
}

// ModelContextResult supplies additional instructions/retrieval material. The
// consumer preserves authored conversation and Tool contracts under their owners.
type ModelContextResult struct {
	Messages []chat.Message `json:"messages"`
}

func ModelContextMethod() (*Method[ModelContextRequest, ModelContextResult], error) {
	return NewMethod[ModelContextRequest, ModelContextResult](MethodConfig{Name: "flame.model-context/contribute.v1", Kind: capability.Query, Target: "call", Description: "Contribute additional system context before the captured model request's capacity admission."})
}

// ModelContextContract orders independent context contributions before the final
// request is admitted. It grants no next proxy, model call or route replacement.
func ModelContextContract() (*capability.Contract, error) {
	method, err := ModelContextMethod()
	if err != nil {
		return nil, err
	}
	return capability.New(capability.Config{Reference: "flame.model-context.v1", Composition: capability.Decoration, Scope: "call"}, method.Descriptor())
}

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
	return r.Request.Validate()
}
