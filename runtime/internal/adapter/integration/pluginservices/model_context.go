package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

func (o *ModelComposition) ResolveContext(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}

func (o *ModelComposition) ValidateContext(ctx context.Context, bindings []plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		call := installedCall{calls: o.calls, request: plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract}}
		captured, err := call.resolve(ctx)
		if err != nil {
			return err
		}
		if captured != binding {
			return plugin.ErrStale
		}
	}
	return nil
}

// PrepareContext executes only captured input contributors. Their output remains
// request context, never authored conversation, model selection or Tool identity.
func (o *ModelComposition) PrepareContext(ctx context.Context, bindings []plugins.ContributionBinding, selection modelref.Selection, request *chat.Request) ([]chat.Message, error) {
	if request == nil {
		return nil, errors.New("model context: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if _, err := pluginsdk.InvocationFromContext(ctx); err != nil {
		return nil, err
	}
	if err := o.ValidateContext(ctx, bindings); err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		return nil, err
	}
	method, err := pluginsdk.ModelContextMethod()
	if err != nil {
		return nil, err
	}
	var material []chat.Message
	frozen := request.Clone()
	for _, binding := range bindings {
		session, err := o.calls.pool.ServiceSession(ctx, binding.ServerID)
		if err != nil {
			return nil, err
		}
		if _, err := o.calls.pool.SessionActor(ctx, binding.ServerID, session); err != nil {
			return nil, err
		}
		exported, err := pluginsdk.NewCapabilityBinding(session, binding.Export, contract)
		if err != nil {
			return nil, err
		}
		operation, err := exported.Method(method.Descriptor().Name)
		if err != nil {
			return nil, err
		}
		input, err := jsoninput.Encode(pluginsdk.ModelContextRequest{Provider: selection.Provider(), Model: selection.Model(), Request: *frozen.Clone()})
		if err != nil {
			return nil, err
		}
		raw, err := operation.Invoke(ctx, "", input)
		if err != nil {
			return nil, err
		}
		var result pluginsdk.ModelContextResult
		if err := json.Unmarshal(raw.JSON(), &result, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if err := result.Validate(); err != nil {
			return nil, err
		}
		for _, message := range result.Messages {
			material = append(material, message.Clone())
		}
		// Later contributors see earlier context in declared order, with the original
		// request and its contracts preserved for final caller capacity admission.
		prefix := 0
		for prefix < len(request.Messages) && request.Messages[prefix].Role == chat.RoleSystem {
			prefix++
		}
		frozen = request.Clone()
		frozen.Messages = append(cloneContextMessages(request.Messages[:prefix]), cloneContextMessages(material)...)
		frozen.Messages = append(frozen.Messages, cloneContextMessages(request.Messages[prefix:])...)
		if err := frozen.Validate(); err != nil {
			return nil, errors.Join(err, errors.New("model context: composed input is invalid"))
		}
	}
	return material, nil
}

func cloneContextMessages(values []chat.Message) []chat.Message {
	result := make([]chat.Message, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}
