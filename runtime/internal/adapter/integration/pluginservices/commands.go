package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"
	framehost "github.com/Tangerg/frame/host"

	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/samber/lo"
)

type CommandCalls struct {
	framework     framehost.Host
	publish       invalidation.Publish
	installations plugins.Store
	selections    plugins.SelectionStore
	compositions  plugins.CompositionStore
	pool          *mcp.Pool
	routing       *plugins.Commands
}

func NewCommandCalls(host framehost.Host, store plugins.Store, selections plugins.SelectionStore, compositions plugins.CompositionStore, pool *mcp.Pool, routing *plugins.Commands, publish invalidation.Publish) (*CommandCalls, error) {
	if !host.Valid() || lo.IsNil(store) || lo.IsNil(selections) || lo.IsNil(compositions) || pool == nil || routing == nil || publish == nil {
		return nil, errors.New("managed plugin: command routing, installation store and connections are required")
	}
	return &CommandCalls{framework: host, publish: publish, installations: store, selections: selections, compositions: compositions, pool: pool, routing: routing}, nil
}

type commandReply[T any] struct {
	value   T
	binding plugins.ContributionBinding
}

func (c *CommandCalls) original(ctx context.Context, record plugin.CommandBinding) (plugins.ContributionBinding, error) {
	snapshot, found, err := c.installations.Get(ctx, record.Provider.InstallationID)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	if !found || snapshot.Selected.Digest() != record.Provider.Digest || !snapshot.Installation.ServerEnabled(record.Server) {
		return plugins.ContributionBinding{}, plugin.ErrUnavailable
	}
	id, err := snapshot.Installation.ServerID(record.Server)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	contribution, found := snapshot.Selected.Contribution(record.Contribution)
	if !found || contribution.Server != record.Server || contribution.Export != record.Export || contribution.Contract != record.Contract {
		return plugins.ContributionBinding{}, plugin.ErrUnavailable
	}
	return plugins.ContributionBinding{InstallationID: record.Provider.InstallationID, ContributionID: record.Contribution, Digest: record.Provider.Digest, ServerName: record.Server, ServerID: id, Export: record.Export, Contract: record.Contract}, nil
}

func invokeResolvedCommand[P, R any](ctx context.Context, c *CommandCalls, method string, input P, validate func(R) error, resolve func(context.Context) (plugins.ContributionBinding, error), collaborators func(context.Context, plugins.ContributionBinding, *sdk.ClientSession) (sdk.ParamsBase, func() error, error)) (commandReply[R], error) {
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return commandReply[R]{}, err
	}
	var value R
	route := commandRoute{method: method, input: admitted, resolve: resolve, target: func(binding plugins.ContributionBinding) framecall.Target {
		return framecall.Target{Scope: extension.ScopeInstallation, Reference: binding.InstallationID.String()}
	}}
	reply, err := invokeRawCommand(ctx, c, route, func(result framevalue.JSON) error {
		if err := json.Unmarshal(result.Bytes(), &value, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if validate != nil {
			return validate(value)
		}
		return nil
	}, collaborators)
	if err != nil {
		var zero R
		value = zero
	}
	return commandReply[R]{value: value, binding: reply.binding}, err
}

func invokeRawCommand(ctx context.Context, c *CommandCalls, route commandRoute, validate func(framevalue.JSON) error, collaborators func(context.Context, plugins.ContributionBinding, *sdk.ClientSession) (sdk.ParamsBase, func() error, error)) (_ commandReply[framevalue.JSON], returned error) {
	var zero commandReply[framevalue.JSON]
	record, err := c.admitCommandRoute(ctx, route, unaryCall)
	if err != nil {
		return zero, err
	}
	ctx, command, err := c.openCommand(ctx, record)
	if err != nil {
		return zero, err
	}
	defer command.close()
	var result framevalue.JSON
	if record.Phase == plugin.CommandPrepared {
		metadata := sdk.ParamsBase{}
		if collaborators != nil {
			var release func() error
			metadata, release, err = collaborators(ctx, command.binding, command.session)
			if err != nil {
				return zero, err
			}
			defer func() { returned = errors.Join(returned, release()) }()
		}
		if err := command.dispatch(ctx); err != nil {
			return zero, err
		}
		result, err = command.method.InvokeWithMetadata(ctx, pluginmcp.CommandID(record.ID), record.Input, metadata)
	} else {
		result, err = command.readReceipt(ctx)
	}
	result, err = command.settle(ctx, result, err, validate)
	if err != nil {
		return zero, err
	}
	return commandReply[framevalue.JSON]{value: result, binding: command.binding}, nil
}

type openedCommand struct {
	calls      *CommandCalls
	record     plugin.CommandBinding
	binding    plugins.ContributionBinding
	session    *sdk.ClientSession
	method     *pluginmcp.Binding
	release    func()
	dispatched bool
}

func (c *CommandCalls) openCommand(ctx context.Context, record plugin.CommandBinding) (context.Context, *openedCommand, error) {
	ctx, err := pluginmcp.WithInvocation(ctx, record.Invocation)
	if err != nil {
		return ctx, nil, err
	}
	command := &openedCommand{calls: c, record: record, release: func() {}}
	fail := func(err error) (context.Context, *openedCommand, error) {
		command.close()
		return ctx, nil, err
	}
	binding, err := c.original(ctx, record)
	if err != nil {
		if command.mayHaveTakenEffect() {
			err = errors.Join(plugin.ErrCommandUnconfirmed, err)
		}
		return fail(err)
	}
	borrowed, err := c.pool.BorrowActivation(ctx, binding.ServerID, record.Activation)
	if err != nil {
		return fail(errors.Join(plugin.ErrCommandUnconfirmed, err))
	}
	ctx = borrowed.Context()
	command.binding, command.session, command.release = binding, borrowed.Session(), func() { _ = borrowed.Close() }
	contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
	if err != nil {
		return fail(errors.Join(plugin.ErrCommandUnconfirmed, err))
	}
	exported, err := pluginmcp.NewCapabilityBinding(command.session, record.Export, contract)
	if err != nil {
		return fail(errors.Join(plugin.ErrCommandUnconfirmed, err))
	}
	method, err := exported.Method(record.Method)
	if err != nil || method.Descriptor().Kind != frame.Command {
		return fail(errors.Join(plugin.ErrCommandUnconfirmed, err, plugin.ErrInvalid))
	}
	command.method = method
	return ctx, command, nil
}

func (o *openedCommand) mayHaveTakenEffect() bool {
	return o.record.Phase != plugin.CommandPrepared || o.dispatched
}

func (o *openedCommand) dispatch(ctx context.Context) error {
	if err := o.calls.routing.Dispatch(ctx, o.record.ID, o.calls.admitDispatch); err != nil {
		return err
	}
	o.dispatched = true
	return nil
}

func (o *openedCommand) readReceipt(ctx context.Context) (framevalue.JSON, error) {
	result, found, err := o.method.ReadReceipt(ctx, pluginmcp.CommandID(o.record.ID), o.record.Input)
	if err == nil && !found {
		err = plugin.ErrCommandUnconfirmed
	}
	return result, err
}

func (o *openedCommand) settle(ctx context.Context, result framevalue.JSON, err error, validate func(framevalue.JSON) error) (framevalue.JSON, error) {
	if _, refused := errors.AsType[*framecall.Refusal](err); err != nil && !refused {
		return framevalue.JSON{}, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	if err == nil {
		if invalid := validate(result); invalid != nil {
			return framevalue.JSON{}, errors.Join(plugin.ErrCommandUnconfirmed, invalid)
		}
	}
	if observeErr := o.calls.routing.Observe(ctx, o.record.ID); observeErr != nil {
		return framevalue.JSON{}, errors.Join(plugin.ErrCommandUnconfirmed, err, observeErr)
	}
	return result, err
}

func (o *openedCommand) close() {
	o.release()
	if o.mayHaveTakenEffect() {
		o.calls.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	}
}

func (c *CommandCalls) admitDispatch(ctx context.Context, record plugin.CommandBinding) error {
	if _, err := c.original(ctx, record); err != nil {
		return err
	}
	binding, err := plugins.DecodeActivation(record.Activation)
	if err != nil {
		return err
	}
	return c.framework.ValidateBindingUnder(ctx, binding)
}

func (c *CommandCalls) NotifyChange(ctx context.Context, contribution plugin.ContributionID) error {
	actor, found := operation.ActorFrom(ctx)
	if !found {
		return plugin.ErrUnavailable
	}
	snapshot, found, err := c.installations.Get(ctx, actor.InstallationID())
	if err != nil {
		return err
	}
	if !found || snapshot.Selected.Digest() != actor.Release() || !snapshot.Installation.ServerEnabled(actor.Server()) {
		return plugin.ErrUnavailable
	}
	owner, found := snapshot.Selected.Contribution(contribution)
	if !found || owner.Server != actor.Server() {
		return plugin.ErrUnavailable
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	return nil
}

// A command route names the method and admitted input, how a new route picks its
// contribution, which invocation target it captures, and what an existing route
// must still satisfy.
type commandRoute struct {
	method  string
	input   framevalue.JSON
	resolve func(context.Context) (plugins.ContributionBinding, error)
	target  func(plugins.ContributionBinding) framecall.Target
	accepts func(plugin.CommandBinding) error
}

func (c *CommandCalls) admitCommandRoute(ctx context.Context, route commandRoute, delivery callDelivery) (plugin.CommandBinding, error) {
	id := operation.Identity(ctx)
	capture := func(binding plugins.ContributionBinding) (framecall.Invocation, error) {
		return operation.Capture(ctx, route.target(binding))
	}
	command, found, err := c.routing.Find(ctx, id, route.method, route.input)
	if err == nil && !found {
		command, err = c.bindCommandRoute(ctx, id, route, delivery, capture)
	}
	if errors.Is(err, plugin.ErrCommandConflict) {
		return plugin.CommandBinding{}, fmt.Errorf("managed plugin: %w", errors.Join(framecall.ErrConflict, err))
	}
	if err != nil {
		return plugin.CommandBinding{}, err
	}
	record := command.Snapshot()
	invocation, err := capture(plugins.ContributionBinding{InstallationID: record.Provider.InstallationID})
	if err != nil {
		return plugin.CommandBinding{}, err
	}
	if invocation != record.Invocation {
		return plugin.CommandBinding{}, errors.Join(framecall.ErrConflict, plugin.ErrCommandConflict)
	}
	if route.accepts != nil {
		if err := route.accepts(record); err != nil {
			return plugin.CommandBinding{}, err
		}
	}
	if record.Phase == plugin.CommandRetired {
		return plugin.CommandBinding{}, plugin.ErrCommandRetired
	}
	return record, nil
}

func (c *CommandCalls) bindCommandRoute(ctx context.Context, id string, route commandRoute, delivery callDelivery, capture func(plugins.ContributionBinding) (framecall.Invocation, error)) (_ *plugin.Command, returned error) {
	candidate, err := route.resolve(ctx)
	if err != nil {
		return nil, err
	}
	borrowed, err := borrowContribution(ctx, c.pool, candidate)
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, borrowed.Close()) }()
	ctx = borrowed.Context()
	required, err := plugincontract.ParseCapability(candidate.Contract.Bytes())
	if err != nil {
		return nil, err
	}
	exported, err := pluginmcp.ResolveCapability(ctx, borrowed.Session(), candidate.Export, required)
	if err != nil {
		return nil, err
	}
	binding, err := exported.Method(route.method)
	if err != nil {
		return nil, err
	}
	if !delivery.admits(binding.Descriptor()) {
		return nil, plugin.ErrInvalid
	}
	if binding.Descriptor().Kind != frame.Command {
		return nil, errors.New("managed plugin: command requires a command contract")
	}
	if err := validateInput(ctx, binding, route.input); err != nil {
		return nil, err
	}
	activation, err := borrowed.Activation()
	if err != nil {
		return nil, err
	}
	originalBinding, err := plugins.DecodeActivation(activation)
	if err != nil {
		return nil, err
	}
	return c.routing.Bind(ctx, id, route.method, route.input, func(ctx context.Context) (plugins.CommandAdmission, error) {
		current, err := route.resolve(ctx)
		if err != nil {
			return plugins.CommandAdmission{}, err
		}
		if !current.SameContribution(candidate) {
			return plugins.CommandAdmission{}, plugin.ErrUnavailable
		}
		if _, err := c.original(ctx, plugin.CommandBinding{Provider: plugin.Dependency{InstallationID: candidate.InstallationID, Digest: candidate.Digest}, Server: candidate.ServerName, Contribution: candidate.ContributionID, Export: candidate.Export, Contract: candidate.Contract}); err != nil {
			return plugins.CommandAdmission{}, err
		}
		if err := c.framework.ValidateBindingUnder(ctx, originalBinding); err != nil {
			return plugins.CommandAdmission{}, plugins.FrameworkError(err)
		}
		return plugins.CommandAdmission{Binding: current, Activation: activation}, nil
	}, capture)
}
