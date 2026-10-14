package pluginservices

import (
	"context"
	"errors"
	"slices"
	"sync"

	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

// Auxiliary freezes context and observer bindings for the caller's whole use case. Memory
// retains one capture across extraction/curation; Complete releases one request.
func (o *ModelComposition) Auxiliary(resolve models.AuxiliaryResolver) models.AuxiliaryResolver {
	return func(ctx context.Context, budget models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		if err := budget.Validate(); err != nil {
			return models.SelectedChat{}, nil, err
		}
		if resolve == nil {
			return models.SelectedChat{}, nil, errors.New("model observers: auxiliary resolver is required")
		}
		resolved, release, err := resolve(ctx, budget)
		if err != nil {
			return models.SelectedChat{}, nil, err
		}
		if release == nil {
			return models.SelectedChat{}, nil, errors.New("model observers: auxiliary model release is required")
		}
		if dependency.Missing(resolved.Model()) {
			release()
			return models.SelectedChat{}, nil, errors.New("model observers: auxiliary model is required")
		}
		var capture *auxiliaryModelCapture
		err = o.admission.Under(ctx, func() error {
			bindings, err := o.Resolve(ctx)
			if err != nil {
				return err
			}
			contexts, err := o.ResolveContext(ctx)
			if err != nil {
				return err
			}
			inputs, err := o.ResolveInput(ctx)
			if err != nil {
				return err
			}
			bound, _ := o.Bind(resolved.Model(), nil, bindings)
			lifetime, cancel := context.WithCancelCause(ctx)
			capture = &auxiliaryModelCapture{owner: o, bindings: bindings, contexts: contexts, inputs: inputs, resolved: resolved, budget: budget, model: bound, lifetime: lifetime, cancel: cancel, release: release, done: make(chan struct{})}
			o.mu.Lock()
			o.auxiliary[capture] = struct{}{}
			o.mu.Unlock()
			return nil
		})
		if err != nil {
			release()
			return models.SelectedChat{}, nil, err
		}
		result, err := resolved.WithModel(capture)
		if err != nil {
			capture.close()
			return models.SelectedChat{}, nil, err
		}
		return result, capture.close, nil
	}
}

func (o *ModelComposition) HeldDependencies() []plugin.Dependency {
	o.mu.Lock()
	defer o.mu.Unlock()
	var values []plugin.Dependency
	for capture := range o.auxiliary {
		for _, binding := range append(append(slices.Clone(capture.bindings), capture.contexts...), capture.inputs...) {
			values = append(values, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest})
		}
	}
	return plugin.CompactDependencies(values)
}

type auxiliaryModelCapture struct {
	owner    *ModelComposition
	bindings []plugins.ContributionBinding
	contexts []plugins.ContributionBinding
	inputs   []plugins.ContributionBinding
	resolved models.SelectedChat
	budget   models.AuxiliaryBudget
	model    chat.Model
	lifetime context.Context
	cancel   context.CancelCauseFunc
	release  func()
	mu       sync.Mutex
	closed   bool
	active   sync.WaitGroup
	done     chan struct{}
}

func (c *auxiliaryModelCapture) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Target.Scope != "call" {
		return nil, errors.Join(err, errors.New("model observers: auxiliary call identity is required"))
	}
	c.mu.Lock()
	if c.closed || c.lifetime.Err() != nil {
		c.mu.Unlock()
		return nil, errors.Join(pluginsdk.ErrStaleCall, c.lifetime.Err(), context.Cause(c.lifetime))
	}
	c.active.Add(1)
	c.mu.Unlock()
	defer c.active.Done()
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(c.lifetime, func() { cancel(context.Cause(c.lifetime)) })
	defer stop()
	defer cancel(nil)
	prepared, err := c.resolved.PrepareRequest(request)
	if err != nil {
		return nil, err
	}
	request = prepared
	if err := c.admit(request); err != nil {
		return nil, err
	}
	material, err := c.owner.PrepareContext(calling, c.contexts, c.resolved.Selection(), request)
	if err != nil {
		return nil, err
	}
	sent := request.Clone()
	prefix := 0
	for prefix < len(request.Messages) && request.Messages[prefix].Role == chat.RoleSystem {
		prefix++
	}
	sent.Messages = append(cloneContextMessages(request.Messages[:prefix]), material...)
	sent.Messages = append(sent.Messages, cloneContextMessages(request.Messages[prefix:])...)
	sent, err = c.owner.PrepareInput(calling, c.inputs, c.resolved.Selection(), sent)
	if err != nil {
		return nil, err
	}
	if err := c.admit(sent); err != nil {
		return nil, err
	}
	if err := c.resolved.AdmitRequest(calling, sent); err != nil {
		return nil, err
	}
	return c.model.Call(calling, sent)
}

func (c *auxiliaryModelCapture) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.cancel(pluginsdk.ErrStaleCall)
	c.active.Wait()
	c.release()
	o := c.owner
	o.mu.Lock()
	delete(o.auxiliary, c)
	o.mu.Unlock()
	close(c.done)
}

func (c *auxiliaryModelCapture) admit(request *chat.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens <= 0 || *request.Options.MaxOutputTokens > c.budget.MaxOutputTokens {
		return errors.New("model composition: auxiliary output exceeds the captured budget")
	}
	if len(request.Tools) > 0 || request.ToolChoice != nil {
		return errors.New("model composition: auxiliary text generation cannot advertise Tools")
	}
	remaining := c.budget.MaxInputBytes
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.Kind != chat.PartText {
				return errors.New("model composition: auxiliary input must contain text")
			}
			if len(part.Text) > remaining {
				return errors.New("model composition: auxiliary input exceeds the captured budget")
			}
			remaining -= len(part.Text)
		}
	}
	return nil
}
