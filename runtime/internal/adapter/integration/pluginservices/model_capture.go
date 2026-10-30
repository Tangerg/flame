package pluginservices

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// capture retains the exact contributions until all provider work has joined.
// Admission policy belongs to the calling use case, not to this lifetime owner.
func (o *CallComposition) capture(ctx context.Context, resolved models.SelectedChat, release func() error, admit func(*chat.Request) error) (models.SelectedChat, func() error, error) {
	if release == nil {
		return models.SelectedChat{}, nil, errors.New("model composition: model release is required")
	}
	if lo.IsNil(resolved.Model()) || admit == nil {
		return models.SelectedChat{}, nil, errors.Join(errors.New("model composition: model and admission owner are required"), release())
	}
	var capture *modelCapture
	err := o.admission.Admit(ctx, func(ctx context.Context) error {
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
		activations, err := o.activationsUnder(ctx, modelCompositionBindings(resolved, bindings, contexts, inputs))
		if err != nil {
			return err
		}
		bindings, err = plugins.BindActivations(bindings, activations)
		if err != nil {
			return err
		}
		contexts, err = plugins.BindActivations(contexts, activations)
		if err != nil {
			return err
		}
		inputs, err = plugins.BindActivations(inputs, activations)
		if err != nil {
			return err
		}
		streamer, _ := resolved.Model().(chat.Streamer)
		bound, stream := o.Bind(resolved.Model(), streamer, bindings)
		lifetime, cancel := context.WithCancelCause(ctx)
		capture = &modelCapture{activations: activations, owner: o, bindings: bindings, contexts: contexts, inputs: inputs, resolved: resolved, admit: admit, model: bound, stream: stream, lifetime: lifetime, cancel: cancel, release: release, done: make(chan struct{})}
		o.mu.Lock()
		o.captures[capture] = struct{}{}
		o.mu.Unlock()
		return nil
	})
	if err != nil {
		return models.SelectedChat{}, nil, errors.Join(err, release())
	}
	var model chat.Model = capture
	if capture.stream != nil {
		model = &streamedModelCapture{capture}
	}
	result, err := resolved.WithModel(model)
	if err != nil {
		return models.SelectedChat{}, nil, errors.Join(err, capture.close())
	}
	return result, capture.close, nil
}

func (o *CallComposition) Activations(ctx context.Context, bindings []plugins.ContributionBinding) ([]plugin.Activation, error) {
	var activations []plugin.Activation
	err := o.admission.Admit(ctx, func(ctx context.Context) error {
		var err error
		activations, err = o.activationsUnder(ctx, bindings)
		return err
	})
	return activations, err
}

func (o *CallComposition) activationsUnder(ctx context.Context, bindings []plugins.ContributionBinding) ([]plugin.Activation, error) {
	var activations []plugin.Activation
	for _, binding := range bindings {
		var activation plugin.Activation
		var err error
		if binding.Activation != nil {
			activation, err = plugins.OriginalActivation([]plugin.Activation{*binding.Activation}, binding.InstallationID, binding.Digest, binding.ServerName)
		} else {
			activation, err = o.calls.pool.InstallationActivation(ctx, binding.ServerID)
		}
		if err != nil {
			return nil, err
		}
		if !slices.Contains(activation.Dependencies, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest}) {
			return nil, plugin.ErrStale
		}
		if slices.ContainsFunc(activations, activation.Equal) {
			continue
		}
		original, err := plugins.DecodeActivation(activation)
		if err != nil {
			return nil, err
		}
		if err := o.calls.framework.ValidateBindingUnder(ctx, original); err != nil {
			return nil, plugins.FrameworkError(err)
		}
		activations = append(activations, activation)
	}
	return activations, nil
}

// Recovery must not require a live session while its existing connection owner
// is still admitting one. A blocked handshake cannot block Runtime management.
func (o *CallComposition) Pending(bindings []plugins.ContributionBinding) bool {
	for _, status := range o.calls.pool.Statuses() {
		if status.State == mcpserver.ConnectionConnecting && slices.ContainsFunc(bindings, func(binding plugins.ContributionBinding) bool { return binding.ServerID == status.Server }) {
			return true
		}
	}
	return false
}

func (o *CallComposition) HeldDependencies() []plugin.Dependency {
	o.mu.Lock()
	defer o.mu.Unlock()
	var values []plugin.Dependency
	for capture := range o.captures {
		for _, activation := range capture.activations {
			values = append(values, activation.Dependencies...)
		}
	}
	return plugin.CompactDependencies(values)
}

type modelCapture struct {
	activations []plugin.Activation
	owner       *CallComposition
	bindings    []plugins.ContributionBinding
	contexts    []plugins.ContributionBinding
	inputs      []plugins.ContributionBinding
	resolved    models.SelectedChat
	admit       func(*chat.Request) error
	model       chat.Model
	stream      chat.Streamer
	lifetime    context.Context
	cancel      context.CancelCauseFunc
	release     func() error
	mu          sync.Mutex
	closed      bool
	active      sync.WaitGroup
	done        chan struct{}
	err         error
}

func (c *modelCapture) enter(ctx context.Context) (context.Context, func(), error) {
	invocation, err := qimumcp.InvocationFromContext(ctx)
	if err != nil || invocation.Target.Scope != extension.ScopeCall {
		return nil, nil, errors.Join(err, errors.New("model composition: model call identity is required"))
	}
	c.mu.Lock()
	if c.closed || c.lifetime.Err() != nil {
		c.mu.Unlock()
		return nil, nil, errors.Join(qimumcp.ErrStaleCall, c.lifetime.Err(), context.Cause(c.lifetime))
	}
	c.active.Add(1)
	c.mu.Unlock()
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(c.lifetime, func() { cancel(context.Cause(c.lifetime)) })
	return calling, func() { stop(); cancel(nil); c.active.Done() }, nil
}
func (c *modelCapture) prepare(ctx context.Context, request *chat.Request) (*chat.Request, error) {
	activations, err := c.owner.Activations(ctx, modelCompositionBindings(c.resolved, c.bindings, c.contexts, c.inputs))
	if err != nil {
		return nil, err
	}
	if !slices.EqualFunc(activations, c.activations, plugin.Activation.Equal) {
		return nil, plugin.ErrStale
	}
	prepared, err := c.resolved.PrepareRequest(request)
	if err != nil {
		return nil, err
	}
	request = prepared
	if err := c.admit(request); err != nil {
		return nil, err
	}
	material, err := c.owner.PrepareContext(ctx, c.contexts, c.resolved.Selection(), request, nil)
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
	sent, err = c.owner.PrepareInput(ctx, c.inputs, c.resolved.Selection(), sent)
	if err != nil {
		return nil, err
	}
	if err := c.admit(sent); err != nil {
		return nil, err
	}
	if err := c.resolved.AdmitRequest(ctx, sent); err != nil {
		return nil, err
	}
	return sent, nil
}

func (c *modelCapture) Call(ctx context.Context, request *chat.Request) (response *chat.Response, returned error) {
	calling, leave, err := c.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	calling, finish, err := beginCapturedModelCall(calling, c.resolved.Selection())
	if err != nil {
		return nil, err
	}
	outcome := errors.New("model call: captured call did not return normally")
	defer func() { returned = errors.Join(returned, finish(calling, outcome)) }()
	sent, err := c.prepare(calling, request)
	if err != nil {
		outcome = err
		return nil, err
	}
	response, outcome = c.model.Call(calling, sent)
	return response, outcome
}

type streamedModelCapture struct{ *modelCapture }

func (c *streamedModelCapture) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	request = request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		calling, leave, err := c.enter(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		defer leave()
		calling, finish, err := beginCapturedModelCall(calling, c.resolved.Selection())
		if err != nil {
			yield(nil, err)
			return
		}
		outcome := errors.New("model call: captured stream did not return normally")
		continuing := true
		defer func() {
			if err := finish(calling, errors.Join(outcome, context.Cause(calling))); err != nil && continuing {
				yield(nil, err)
			}
		}()
		sent, err := c.prepare(calling, request)
		if err != nil {
			outcome = err
			continuing = yield(nil, err)
			return
		}
		var observed error
		c.stream.Stream(calling, sent)(func(delta *chat.ResponseDelta, err error) bool {
			observed = errors.Join(observed, err)
			continuing = yield(delta, err)
			return continuing
		})
		outcome = observed
	}
}

func (c *modelCapture) close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return c.err
	}
	c.closed = true
	c.mu.Unlock()
	c.cancel(qimumcp.ErrStaleCall)
	c.active.Wait()
	c.err = c.release()
	o := c.owner
	o.mu.Lock()
	delete(o.captures, c)
	o.mu.Unlock()
	close(c.done)
	return c.err
}

func beginCapturedModelCall(ctx context.Context, selection modelchoice.Selection) (context.Context, func(context.Context, error) error, error) {
	invocation, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	if invocation.ID == executionctx.InvocationOwner(ctx) {
		return nil, nil, errors.New("model call: independent invocation cannot claim its executing owner identity")
	}
	publisher := executionctx.ModelCallsFromContext(ctx)
	if publisher == nil {
		if executionctx.InvocationOwner(ctx) != "" {
			return nil, nil, errors.New("model call: executing owner has no model consumption publisher")
		}
		return ctx, func(context.Context, error) error { return nil }, nil
	}
	finish, err := publisher.Start(ctx, invocation.ID, selection)
	if err != nil {
		return nil, nil, err
	}
	return executionctx.WithInvocationOwner(ctx, invocation.ID), finish, nil
}

func modelCompositionBindings(resolved models.SelectedChat, observers, contexts, inputs []plugins.ContributionBinding) []plugins.ContributionBinding {
	bindings := slices.Concat(observers, contexts, inputs)
	if capture, found := resolved.Capture(); found {
		bindings = append(bindings, capture.Binding)
	}
	return bindings
}
