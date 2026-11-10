package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
)

// ModelCallBudget bounds the complete request, including context, Tools and media.
// Estimates are admission evidence; they never become reported usage or cost.
type ModelCallBudget struct {
	MaxRequestBytes int64 `json:"maxRequestBytes" jsonschema:"minimum=1"`
	MaxOutputTokens int64 `json:"maxOutputTokens" jsonschema:"minimum=1"`
}

func (b ModelCallBudget) Admit(request *chat.Request) error {
	if b.MaxRequestBytes <= 0 || b.MaxOutputTokens <= 0 {
		return errors.New("model call: positive request/output budget is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens <= 0 || *request.Options.MaxOutputTokens > b.MaxOutputTokens {
		return errors.New("model call: output exceeds the admitted budget")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if int64(len(encoded)) > b.MaxRequestBytes {
		return errors.New("model call: request exceeds the admitted budget")
	}
	return nil
}

type ModelCallRequest struct {
	Selection ModelSelection  `json:"selection"`
	Budget    ModelCallBudget `json:"budget"`
	Request   chat.Request    `json:"request"`
}

func (r ModelCallRequest) Validate() error {
	if err := r.Selection.Validate(); err != nil {
		return err
	}
	if err := r.Budget.Admit(&r.Request); err != nil {
		return err
	}
	if r.Request.Options.Model != "" && r.Request.Options.Model != r.Selection.Model {
		return errors.New("model call: request model differs from proposed selection")
	}
	if r.Request.Options.ReasoningEffort != "" && string(r.Request.Options.ReasoningEffort) != r.Selection.ReasoningEffort {
		return errors.New("model call: request reasoning differs from proposed selection")
	}
	return nil
}

func SelectedModelMethod() (*pluginmcp.Method[ModelCallRequest, ModelResult], error) {
	return pluginmcp.NewMethod[ModelCallRequest, ModelResult](portable.MethodConfig{Name: "flame.model/selected-call.v1", Kind: portable.Command, Target: extension.ScopeCall, Description: "Perform one independently selected, budgeted and observed model exchange."})
}
func SelectedModelStreamMethod() (*pluginmcp.StreamingMethod[ModelCallRequest, scopewire.ResponseDelta, ModelResult], error) {
	return pluginmcp.NewStreamingMethod[ModelCallRequest, scopewire.ResponseDelta, ModelResult](portable.MethodConfig{Name: "flame.model/selected-stream.v1", Kind: portable.Command, Target: extension.ScopeCall, Description: "Consume one independently selected, budgeted and observed model stream."})
}
func CallSelectedModel(ctx context.Context, ref pluginmcp.CallbackRef, id pluginmcp.CommandID, input ModelCallRequest) (*chat.Response, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	input.Request = *input.Request.Clone()
	method, err := SelectedModelMethod()
	if err != nil {
		return nil, err
	}
	result, err := pluginmcp.CallHost(ctx, ref, method, pluginmcp.CommandID(id), input)
	if err != nil {
		return nil, err
	}
	return modelOutcome(result)
}
func StreamSelectedModel(ctx context.Context, ref pluginmcp.CallbackRef, id pluginmcp.CommandID, input ModelCallRequest) iter.Seq2[pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult], error] {
	method, err := SelectedModelStreamMethod()
	if err == nil {
		err = input.Validate()
	}
	if err != nil {
		return func(yield func(pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult], error) bool) {
			yield(pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult]{}, err)
		}
	}
	input.Request = *input.Request.Clone()
	return pluginmcp.StreamHost(ctx, ref, method, pluginmcp.CommandID(id), input)
}
func NewSelectedModelCallback(call func(context.Context, pluginmcp.Command[ModelCallRequest]) (*chat.Response, error)) (pluginmcp.Callback, error) {
	if call == nil {
		return pluginmcp.Callback{}, errors.New("plugin: selected model call owner is required")
	}
	method, err := SelectedModelMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	return pluginmcp.NewCallback(method, nil, func(ctx context.Context, command pluginmcp.Command[ModelCallRequest]) (ModelResult, error) {
		if err := command.Input.Validate(); err != nil {
			return ModelResult{}, err
		}
		command.Input.Request = *command.Input.Request.Clone()
		ctx, err := selectedModelCallContext(ctx, string(command.ID))
		if err != nil {
			return ModelResult{}, err
		}
		response, err := call(ctx, command)
		if refusal, known := errors.AsType[*framemcp.Refusal](err); known && response == nil {
			return ModelResult{}, refusal
		}
		return modelResult(response, err)
	})
}
func NewSelectedModelStreamCallback(stream func(context.Context, pluginmcp.Command[ModelCallRequest]) iter.Seq2[*chat.ResponseDelta, error]) (pluginmcp.Callback, error) {
	if stream == nil {
		return pluginmcp.Callback{}, errors.New("plugin: selected model stream owner is required")
	}
	method, err := SelectedModelStreamMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	return pluginmcp.NewStreamingCallback(method, func(ctx context.Context, command pluginmcp.Command[ModelCallRequest]) iter.Seq2[pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult], error] {
		return func(yield func(pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult], error) bool) {
			if err := command.Input.Validate(); err != nil {
				yield(pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult]{}, err)
				return
			}
			command.Input.Request = *command.Input.Request.Clone()
			ctx, err := selectedModelCallContext(ctx, string(command.ID))
			if err != nil {
				yield(pluginmcp.StreamValue[scopewire.ResponseDelta, ModelResult]{}, err)
				return
			}
			modelStreamResult(ctx, func() iter.Seq2[*chat.ResponseDelta, error] { return stream(ctx, command) })(yield)
		}
	})
}

func selectedModelCallContext(ctx context.Context, id string) (context.Context, error) {
	parent, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return nil, errors.Join(err, errors.New("plugin: selected model requires an owning invocation"))
	}
	if parent.ID == id {
		return nil, errors.New("plugin: independent model invocation requires a distinct identity")
	}
	return modelCallContext(ctx, id)
}
