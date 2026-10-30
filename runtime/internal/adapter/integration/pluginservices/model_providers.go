package pluginservices

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type ModelProvidersConfig struct {
	Lifetime      context.Context
	Framework     *qimu.Host
	Admission     *qimu.Admission
	Installations plugins.Store
	Selections    plugins.SelectionStore
	Connections   *mcp.Pool
	Callbacks     *qimumcp.Callbacks
}

// ModelProviders retains admitted model constructions. The private construction
// inputs never enter the public plugin-command projection or a Run checkpoint.
type ModelProviders struct {
	config   ModelProvidersConfig
	contract *portable.Capability
	mu       sync.Mutex
	uses     map[*providerUse]struct{}
	closed   bool
}

func NewModelProviders(config ModelProvidersConfig) (*ModelProviders, error) {
	if config.Lifetime == nil || config.Framework == nil || config.Admission == nil || lo.IsNil(config.Installations) || lo.IsNil(config.Selections) || config.Connections == nil || config.Callbacks == nil {
		return nil, errors.New("model providers: lifetime, installed sources, admission, connections and callbacks are required")
	}
	contract, err := modelprovider.Contract()
	if err != nil {
		return nil, err
	}
	owner := &ModelProviders{config: config, contract: contract, uses: make(map[*providerUse]struct{})}
	if err := config.Admission.RegisterUseOwner(owner); err != nil {
		return nil, err
	}
	return owner, nil
}

func (*ModelProviders) ID() string { return "flame.model-provider-captures" }

func (p *ModelProviders) Held(_ context.Context, installation qimu.InstallationID) ([]qimu.Dependency, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result []qimu.Dependency
	for use := range p.uses {
		for _, dependency := range use.binding.Activation.Dependencies {
			if dependency.InstallationID.String() == installation.String() {
				value := qimu.Dependency{InstallationID: installation, Release: qimu.Digest(dependency.Digest.String())}
				if !slices.Contains(result, value) {
					result = append(result, value)
				}
			}
		}
	}
	return result, nil
}

func (p *ModelProviders) acquire(ctx context.Context, original *plugins.ContributionBinding) (*providerUse, error) {
	var binding plugins.ContributionBinding
	if original != nil {
		binding = *original
	} else {
		err := p.config.Admission.Admit(ctx, func(ctx context.Context) error {
			selected, found, err := plugins.SelectedContribution(ctx, p.config.Framework, p.config.Installations, p.config.Selections, p.contract)
			if err != nil {
				return err
			}
			if !found {
				return plugin.ErrUnavailable
			}
			binding = selected
			return nil
		})
		if err != nil {
			return nil, err
		}
		activation, err := p.config.Connections.AwaitInstallationActivation(ctx, binding.ServerID)
		if err != nil {
			if errors.Is(err, mcp.ErrUnavailable) {
				err = errors.Join(plugin.ErrUnavailable, err)
			}
			return nil, err
		}
		activation, err = plugins.OriginalActivation([]plugin.Activation{activation}, binding.InstallationID, binding.Digest, binding.ServerName)
		if err != nil {
			return nil, err
		}
		binding.Activation = &activation
	}
	var use *providerUse
	err := p.config.Admission.Admit(ctx, func(ctx context.Context) error {
		if binding.Activation == nil || binding.Contract != p.contract.Identity() {
			return plugin.ErrInvalid
		}
		captured, err := plugins.DecodeActivation(*binding.Activation)
		if err != nil {
			return err
		}
		if err := p.config.Framework.ValidateBindingUnder(ctx, captured); err != nil {
			return plugins.FrameworkError(err)
		}
		lifetime, cancel := context.WithCancelCause(p.config.Lifetime)
		use = &providerUse{owner: p, binding: binding, lifetime: lifetime, cancel: cancel, done: make(chan struct{})}
		p.mu.Lock()
		if p.closed || p.config.Lifetime.Err() != nil {
			p.mu.Unlock()
			cancel(qimumcp.ErrStaleCall)
			return qimumcp.ErrStaleCall
		}
		p.uses[use] = struct{}{}
		p.mu.Unlock()
		return nil
	})
	return use, err
}

func (p *ModelProviders) Profiles(ctx context.Context) (_ []modelprovider.Profile, returned error) {
	use, err := p.acquire(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, use.Close(ctx)) }()
	method, err := modelprovider.ProfilesMethod()
	if err != nil {
		return nil, err
	}
	profiles, err := providerMethod(ctx, use, method, "", struct{}{})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if err := profile.Validate(); err != nil {
			return nil, err
		}
		if seen[profile.ID] {
			return nil, errors.New("model providers: repeated provider identity")
		}
		seen[profile.ID] = true
	}
	return profiles, nil
}

func (p *ModelProviders) openUse(ctx context.Context, connection modelprovider.Connection) (_ *providerUse, _ modelprovider.Profile, returned error) {
	use, err := p.acquire(ctx, nil)
	if err != nil {
		return nil, modelprovider.Profile{}, err
	}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, use.Close(ctx))
		}
	}()
	method, err := modelprovider.ProfilesMethod()
	if err != nil {
		return nil, modelprovider.Profile{}, err
	}
	profiles, err := providerMethod(ctx, use, method, "", struct{}{})
	if err != nil {
		return nil, modelprovider.Profile{}, err
	}
	index := slices.IndexFunc(profiles, func(profile modelprovider.Profile) bool { return profile.ID == connection.Provider })
	if index < 0 {
		return nil, modelprovider.Profile{}, modelapp.ErrProviderUnsupported
	}
	profile := profiles[index]
	if err := profile.Validate(); err != nil {
		return nil, modelprovider.Profile{}, err
	}
	if profile.Authentication == modelprovider.KeyRequired && connection.APIKey == "" {
		return nil, modelprovider.Profile{}, modelapp.ErrProviderUnconfigured
	}
	if profile.EndpointRequired && connection.BaseURL == "" {
		return nil, modelprovider.Profile{}, modelapp.ErrProviderBaseURLRequired
	}
	return use, profile, nil
}

func (p *ModelProviders) OpenChat(ctx context.Context, selection modelchoice.Selection, connection modelprovider.Connection) (_ models.ResolvedChat, returned error) {
	use, profile, err := p.openUse(ctx, connection)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, use.Close(ctx))
		}
	}()
	ctx, leave, err := use.enter(ctx)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	defer leave()
	method, err := modelprovider.OpenChatMethod()
	if err != nil {
		return models.ResolvedChat{}, err
	}
	id := uuid.NewString()
	opened, err := providerMethod(ctx, use, method, id, modelprovider.Open{Connection: connection, Model: selection.Model()})
	if err != nil {
		return models.ResolvedChat{}, err
	}
	use.handle = opened.Handle
	if opened.Original != id {
		return models.ResolvedChat{}, errors.New("model providers: original construction identity changed")
	}
	return use.chat(selection, profile, opened)
}

func (p *ModelProviders) RestoreChat(ctx context.Context, selection modelchoice.Selection, original *models.ProviderCapture) (_ models.ResolvedChat, returned error) {
	if original == nil {
		return models.ResolvedChat{}, errors.New("model providers: original construction is required")
	}
	capture := original.Clone()
	if err := capture.ValidateSelection(selection); err != nil {
		return models.ResolvedChat{}, err
	}
	use, err := p.acquire(ctx, &capture.Binding)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, use.Close(ctx))
		}
	}()
	ctx, leave, err := use.enter(ctx)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	defer leave()
	method, err := modelprovider.ReopenChatMethod()
	if err != nil {
		return models.ResolvedChat{}, err
	}
	opened, err := providerMethod(ctx, use, method, uuid.NewString(), modelprovider.Reopen{Original: capture.Chat.Original})
	if err != nil {
		return models.ResolvedChat{}, err
	}
	use.handle = opened.Handle
	if opened.Original != capture.Chat.Original || opened.Streaming != capture.Chat.Streaming || opened.TokenCounting != capture.Chat.TokenCounting {
		return models.ResolvedChat{}, errors.New("model providers: original construction capabilities changed")
	}
	expected := capture.Chat
	expected.Handle = opened.Handle
	originalFacts, err := jsonvalue.Encode(expected)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	restoredFacts, err := jsonvalue.Encode(opened)
	if err != nil {
		return models.ResolvedChat{}, err
	}
	if originalFacts != restoredFacts {
		return models.ResolvedChat{}, errors.New("model providers: original construction facts changed")
	}
	return use.chat(selection, capture.Profile, opened)
}

func (u *providerUse) chat(selection modelchoice.Selection, profile modelprovider.Profile, opened modelprovider.Chat) (models.ResolvedChat, error) {
	call, err := modelprovider.CallMethod()
	if err != nil {
		return models.ResolvedChat{}, err
	}
	u.requestMethods = []string{call.Name()}
	if opened.Streaming {
		stream, err := modelprovider.StreamMethod()
		if err != nil {
			return models.ResolvedChat{}, err
		}
		u.requestMethods = append(u.requestMethods, stream.Descriptor().Name)
	}
	if opened.TokenCounting {
		count, err := modelprovider.CountMethod()
		if err != nil {
			return models.ResolvedChat{}, err
		}
		u.requestMethods = append(u.requestMethods, count.Name())
	}
	base := &providerChat{use: u}
	var model chat.Model = base
	if opened.Streaming {
		model = &streamingProviderChat{providerChat: base}
	}
	var counter models.InputTokenCounter
	if opened.TokenCounting {
		counter = &providerCounter{use: u}
	}
	return models.NewCapturedChat(selection, models.ProviderCapture{Binding: u.binding, Profile: profile, Chat: opened}, model, counter, u.Close, u.futureRequestFits)
}

func (p *ModelProviders) Close(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	uses := make([]*providerUse, 0, len(p.uses))
	for use := range p.uses {
		uses = append(uses, use)
	}
	p.mu.Unlock()
	var result error
	for _, use := range uses {
		result = errors.Join(result, use.Close(ctx))
	}
	return result
}

type providerUse struct {
	owner          *ModelProviders
	binding        plugins.ContributionBinding
	handle         modelprovider.Handle
	requestMethods []string
	lifetime       context.Context
	cancel         context.CancelCauseFunc
	mu             sync.Mutex
	closed         bool
	active         sync.WaitGroup
	done           chan struct{}
	err            error
}

func (u *providerUse) enter(ctx context.Context) (context.Context, func(), error) {
	u.mu.Lock()
	if u.closed || u.lifetime.Err() != nil {
		u.mu.Unlock()
		return nil, nil, qimumcp.ErrStaleCall
	}
	u.active.Add(1)
	u.mu.Unlock()
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(u.lifetime, func() { cancel(context.Cause(u.lifetime)) })
	return calling, func() { stop(); cancel(nil); u.active.Done() }, nil
}

func (u *providerUse) Close(ctx context.Context) error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		<-u.done
		return u.err
	}
	u.closed = true
	u.mu.Unlock()
	u.cancel(qimumcp.ErrStaleCall)
	u.active.Wait()
	if u.handle.ID != "" {
		method, err := modelprovider.CloseMethod()
		if err == nil {
			_, err = invokeProvider(ctx, u, method, uuid.NewString(), u.handle)
		}
		u.err = err
	}
	u.owner.mu.Lock()
	delete(u.owner.uses, u)
	u.owner.mu.Unlock()
	close(u.done)
	return u.err
}

func providerMethod[P, R any](ctx context.Context, use *providerUse, method *qimumcp.Method[P, R], id string, input P) (R, error) {
	var zero R
	ctx, leave, err := use.enter(ctx)
	if err != nil {
		return zero, err
	}
	defer leave()
	return invokeProvider(ctx, use, method, id, input)
}

func invokeProvider[P, R any](ctx context.Context, use *providerUse, method *qimumcp.Method[P, R], id string, input P) (_ R, returned error) {
	var zero R
	ctx, err := providerContext(ctx, use.binding, id)
	if err != nil {
		return zero, err
	}
	borrowed, err := borrowContribution(ctx, use.owner.config.Connections, use.binding)
	if err != nil {
		return zero, err
	}
	defer func() { returned = errors.Join(returned, borrowed.Close()) }()
	exported, err := qimumcp.NewCapabilityBinding(borrowed.Session(), use.binding.Export, use.owner.contract)
	if err != nil {
		return zero, err
	}
	bound, err := method.Bind(exported)
	if err != nil {
		return zero, err
	}
	if id == "" {
		return bound.Call(borrowed.Context(), input)
	}
	value, err := bound.CallCommand(borrowed.Context(), id, input)
	if err == nil {
		return value, nil
	}
	original, found, receiptErr := bound.ReadReceipt(borrowed.Context(), id, input)
	if found {
		return original, receiptErr
	}
	return zero, errors.Join(err, receiptErr)
}

func providerContext(ctx context.Context, binding plugins.ContributionBinding, id string) (context.Context, error) {
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		if operation.Identity(ctx) == "" {
			ctx = operation.Bind(ctx, "models.provider", "")
		}
		caller, err = operation.Capture(ctx, portable.Target{Scope: extension.ScopeInstallation, Reference: binding.InstallationID.String()})
		if err != nil {
			return nil, err
		}
	}
	if id == "" {
		id = uuid.NewString()
	}
	parent := caller.ID
	if parent == id {
		parent = caller.Parent
	}
	return qimumcp.WithInvocation(ctx, portable.Invocation{ID: id, Origin: caller.Origin, Parent: parent, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: binding.InstallationID.String()}})
}
