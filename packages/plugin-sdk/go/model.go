package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/scope/core/chat"
	"github.com/invopop/jsonschema"
)

type ModelResult struct {
	Response *chat.Response `json:"response,omitzero"`
	Failure  *ModelFailure  `json:"failure,omitzero"`
}

func (ModelResult) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AnyOf = []*jsonschema.Schema{{Required: []string{"response"}}, {Required: []string{"failure"}}}
}

type ModelFailure struct {
	Code    ModelFailureCode `json:"code"`
	Message string           `json:"message" jsonschema:"pattern=\\S"`
}

type ModelFailureCode string

const (
	ModelFailed           ModelFailureCode = "failed"
	ModelCanceled         ModelFailureCode = "canceled"
	ModelDeadlineExceeded ModelFailureCode = "deadline_exceeded"
)

func (ModelFailureCode) JSONSchemaEnum() []string {
	return []string{string(ModelFailed), string(ModelCanceled), string(ModelDeadlineExceeded)}
}

func (f *ModelFailure) Error() string { return fmt.Sprintf("model call %s: %s", f.Code, f.Message) }

func (f *ModelFailure) Is(target error) bool {
	return f.Code == ModelCanceled && target == context.Canceled || f.Code == ModelDeadlineExceeded && target == context.DeadlineExceeded
}

// ModelMethod publishes Scope's complete exchange without a text-only projection.
func ModelMethod() (*Method[chat.Request, ModelResult], error) {
	return NewMethod[chat.Request, ModelResult](MethodConfig{
		Name: "flame.model/call.v1", Kind: capability.Mutation, Target: "call",
		Description: "Perform one complete exchange with the model captured by this call.",
	})
}

func CallModel(ctx context.Context, ref CallbackRef, id string, request *chat.Request) (*chat.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	method, err := ModelMethod()
	if err != nil {
		return nil, err
	}
	result, err := CallHost(ctx, ref, method, id, *request.Clone())
	if err != nil {
		return nil, err
	}
	return modelOutcome(result)
}

// NewModelCallback leaves command admission and resource policy with its caller.
// Each command projects a call-scoped child identity, preserving origin/parent.
func NewModelCallback(call func(context.Context, Command[chat.Request]) (*chat.Response, error)) (Callback, error) {
	if call == nil {
		return Callback{}, errors.New("plugin: model call owner is required")
	}
	method, err := ModelMethod()
	if err != nil {
		return Callback{}, err
	}
	return NewCallback(method, nil, func(ctx context.Context, command Command[chat.Request]) (ModelResult, error) {
		if err := command.Input.Validate(); err != nil {
			return ModelResult{}, err
		}
		command.Input = *command.Input.Clone()
		ctx, err := modelCallContext(ctx, command.ID)
		if err != nil {
			return ModelResult{}, err
		}
		response, err := call(ctx, command)
		if refusal, known := errors.AsType[*capability.Refusal](err); known && response == nil {
			return ModelResult{}, refusal
		}
		return modelResult(response, err)
	})
}

func modelCallContext(ctx context.Context, id string) (context.Context, error) {
	call := capability.Invocation{ID: id, Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: id}}
	if parent, err := InvocationFromContext(ctx); err == nil {
		if parent.ID == id && parent.Target == call.Target {
			return ctx, nil
		}
		call.Origin, call.Parent = parent.Origin, parent.ID
		if parent.ID == id {
			call.Parent = parent.Parent
		}
	}
	return WithInvocation(ctx, call)
}

func modelResult(response *chat.Response, err error) (ModelResult, error) {
	if response != nil {
		if validationErr := response.Validate(); validationErr != nil {
			return ModelResult{}, errors.Join(err, validationErr)
		}
	} else if err == nil {
		return ModelResult{}, errors.New("plugin: model returned no response")
	}
	result := ModelResult{Response: response.Clone()}
	if err != nil {
		code := ModelFailed
		if errors.Is(err, context.Canceled) {
			code = ModelCanceled
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = ModelDeadlineExceeded
		}
		result.Failure = &ModelFailure{Code: code, Message: err.Error()}
	}
	return result, nil
}

func modelOutcome(result ModelResult) (*chat.Response, error) {
	if result.Response != nil {
		if err := result.Response.Validate(); err != nil {
			return nil, err
		}
	}
	if result.Failure != nil {
		return result.Response.Clone(), result.Failure
	}
	if result.Response == nil {
		return nil, errors.New("plugin: model acknowledgement has no outcome")
	}
	return result.Response.Clone(), nil
}
