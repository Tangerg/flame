package pluginservices

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"

	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

// capture retains the exact contributions until all provider work has joined.
// Admission policy belongs to the calling use case, not to this lifetime owner.
func (o *ModelComposition) capture(ctx context.Context, resolved models.SelectedChat, release func(), admit func(*chat.Request) error) (models.SelectedChat, func(), error) {
	if release == nil {
		return models.SelectedChat{}, nil, errors.New("model composition: model release is required")
	}
	if dependency.Missing(resolved.Model()) || admit == nil {
		release()
		return models.SelectedChat{}, nil, errors.New("model composition: model and admission owner are required")
	}
	var capture *modelCapture
	err := o.admission.Under(ctx, func() error {
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
		streamer, _ := resolved.Model().(chat.Streamer)
		bound, stream := o.Bind(resolved.Model(), streamer, bindings)
		lifetime, cancel := context.WithCancelCause(ctx)
		capture = &modelCapture{owner: o, bindings: bindings, contexts: contexts, inputs: inputs, resolved: resolved, admit: admit, model: bound, stream: stream, lifetime: lifetime, cancel: cancel, release: release, done: make(chan struct{})}
		o.mu.Lock()
		o.captures[capture] = struct{}{}
		o.mu.Unlock()
		return nil
	})
	if err != nil {
		release()
		return models.SelectedChat{}, nil, err
	}
	var model chat.Model = capture
	if capture.stream != nil {
		model = &streamedModelCapture{capture}
	}
	result, err := resolved.WithModel(model)
	if err != nil {
		capture.close()
		return models.SelectedChat{}, nil, err
	}
	return result, capture.close, nil
}

func (o *ModelComposition) HeldDependencies() []plugin.Dependency {
	o.mu.Lock()
	defer o.mu.Unlock()
	var values []plugin.Dependency
	for capture := range o.captures {
		for _, binding := range append(append(slices.Clone(capture.bindings), capture.contexts...), capture.inputs...) {
			values = append(values, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest})
		}
	}
	return plugin.CompactDependencies(values)
}

type modelCapture struct {
	owner    *ModelComposition
	bindings []plugins.ContributionBinding
	contexts []plugins.ContributionBinding
	inputs   []plugins.ContributionBinding
	resolved models.SelectedChat
	admit    func(*chat.Request) error
	model    chat.Model
	stream   chat.Streamer
	lifetime context.Context
	cancel   context.CancelCauseFunc
	release  func()
	mu       sync.Mutex
	closed   bool
	active   sync.WaitGroup
	done     chan struct{}
}

func (c *modelCapture) enter(ctx context.Context) (context.Context, func(), error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Target.Scope != "call" {
		return nil, nil, errors.Join(err, errors.New("model composition: model call identity is required"))
	}
	c.mu.Lock()
	if c.closed || c.lifetime.Err() != nil {
		c.mu.Unlock()
		return nil, nil, errors.Join(pluginsdk.ErrStaleCall, c.lifetime.Err(), context.Cause(c.lifetime))
	}
	c.active.Add(1)
	c.mu.Unlock()
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(c.lifetime, func() { cancel(context.Cause(c.lifetime)) })
	return calling, func() { stop(); cancel(nil); c.active.Done() }, nil
}
func (c *modelCapture) prepare(ctx context.Context, request *chat.Request) (*chat.Request, error) {
	prepared, err := c.resolved.PrepareRequest(request)
	if err != nil {
		return nil, err
	}
	request = prepared
	if err := c.admit(request); err != nil {
		return nil, err
	}
	material, err := c.owner.PrepareContext(ctx, c.contexts, c.resolved.Selection(), request)
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

func (c *modelCapture) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	calling, leave, err := c.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	sent, err := c.prepare(calling, request)
	if err != nil {
		return nil, err
	}
	return c.model.Call(calling, sent)
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
		sent, err := c.prepare(calling, request)
		if err != nil {
			yield(nil, err)
			return
		}
		c.stream.Stream(calling, sent)(yield)
	}
}

func (c *modelCapture) close() {
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
	delete(o.captures, c)
	o.mu.Unlock()
	close(c.done)
}
