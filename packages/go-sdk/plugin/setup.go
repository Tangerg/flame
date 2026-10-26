package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const setupMethod = "io.github.tangerg.flame/operations.setup"
const activateMethod = "io.github.tangerg.flame/operations.activate"

type SetupContribution struct {
	Background *extension.Target `json:"background,omitzero"`
	ID         string            `json:"id"`
	Export     string            `json:"export"`
	Requires   []SetupDependency `json:"requires"`
}

type SetupDependency struct {
	ID       string                         `json:"id"`
	Contract extension.CapabilityDescriptor `json:"contract"`
	Optional bool                           `json:"optional"`
	Bindings []SetupBinding                 `json:"bindings"`
}

// SetupBinding is the setup transport projection of one resolved implementation.
type SetupBinding struct {
	InstallationID string                         `json:"installationId"`
	Digest         string                         `json:"digest"`
	Contribution   string                         `json:"contribution"`
	Contract       extension.CapabilityDescriptor `json:"contract"`
}

// Setup supplies only the entry's named, already resolved dependencies.
// Its callback metadata is ephemeral and never part of business input.
type Setup struct{ contributions []SetupContribution }

func (s Setup) Contributions() ([]SetupContribution, error) {
	data, err := json.Marshal(s.contributions)
	if err != nil {
		return nil, err
	}
	var values []SetupContribution
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	return values, nil
}

type setupParams struct {
	sdk.ParamsBase
	Protocol      string              `json:"protocol"`
	Contributions []SetupContribution `json:"contributions"`
}

func (p *setupParams) UnmarshalJSON(data []byte) error {
	type wire setupParams
	var value wire
	if err := json.Unmarshal(data, &value, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if value.Protocol != extension.Protocol || value.Contributions == nil {
		return errors.New("plugin: current setup protocol and contributions are required")
	}
	*p = setupParams(value)
	return nil
}

// ActivationResource starts business work only after successful setup.
type ActivationResource interface {
	io.Closer
	Start() error
}

type setupActivation struct {
	mu             sync.Mutex
	lifetime       context.Context
	startAttempted bool
	startErr       error
	closed         bool
	setupErr       error
	cancel         context.CancelFunc
	ready          chan struct{}
	done           chan struct{}
	resource       io.Closer
	err            error
	once           sync.Once
}

func (a *setupActivation) close() error {
	a.cancel()
	<-a.ready
	a.once.Do(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.closed = true
		defer close(a.done)
		defer func() {
			if recovered := recover(); recovered != nil {
				a.err = errors.Join(a.err, fmt.Errorf("plugin: setup resource close panicked: %v\n%s", recovered, debug.Stack()))
			}
		}()
		if a.resource != nil {
			a.err = errors.Join(a.err, a.resource.Close())
		}
	})
	<-a.done
	return a.err
}

// Close cancels and joins every setup lifetime before releasing its resources.
func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		<-b.closeDone
		return b.closeErr
	}
	b.closed = true
	sessions := make(map[*sdk.ServerSession]*setupActivation, len(b.activations))
	for session, activation := range b.activations {
		sessions[session] = activation
		activation.cancel()
	}
	b.mu.Unlock()
	var transportErr error
	for session, activation := range sessions {
		transportErr = errors.Join(transportErr, session.Close())
		_ = activation.close()
	}
	b.setupJoins.Wait()
	b.mu.Lock()
	b.closeErr = errors.Join(b.closeErr, transportErr)
	b.mu.Unlock()
	close(b.closeDone)
	return b.closeErr
}

func (b *Backend) OnSetup(handler func(context.Context, Setup) (io.Closer, error)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published || b.closed || handler == nil || b.setup != nil {
		return errors.New("plugin: setup must be registered once before publication")
	}
	b.setup = handler
	return nil
}

func (b *Backend) registerSetup() error {
	if err := b.registerActivation(); err != nil {
		return err
	}
	return sdk.AddReceivingCustomMethod(b.server, setupMethod, func(ctx context.Context, session *sdk.ServerSession, params *setupParams) (*methodResult[bool], error) {
		var err error
		if err := validateSetup(params.Contributions); err != nil {
			return nil, err
		}
		ctx, err = bindCallbackMetadata(ctx, params.Meta)
		if err != nil {
			return nil, err
		}
		b.mu.Lock()
		if !b.published || b.closed {
			b.mu.Unlock()
			return nil, errors.New("plugin: backend setup is unavailable")
		}
		if b.activations[session] != nil {
			b.mu.Unlock()
			return nil, errors.New("plugin: backend session is already set up")
		}
		for _, value := range params.Contributions {
			if b.capabilities[value.Export] == nil {
				b.mu.Unlock()
				return nil, errors.New("plugin: setup contribution export is not registered")
			}
			if (len(value.Requires) > 0 || value.Background != nil) && b.setup == nil {
				b.mu.Unlock()
				return nil, errors.New("plugin: declared dependencies or background role require a setup handler")
			}
		}
		lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
		activation := &setupActivation{lifetime: lifetime, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
		b.activations[session] = activation
		b.setupJoins.Add(1)
		handler := b.setup
		b.mu.Unlock()
		go func() {
			defer b.setupJoins.Done()
			_ = session.Wait()
			closeErr := activation.close()
			b.mu.Lock()
			b.closeErr = errors.Join(b.closeErr, closeErr)
			delete(b.activations, session)
			b.mu.Unlock()
		}()
		stop := context.AfterFunc(ctx, cancel)
		if handler != nil {
			activation.resource, err = invokeSetup(handler, lifetime, Setup{contributions: params.Contributions})
		}
		if err == nil {
			for _, value := range params.Contributions {
				if value.Background != nil {
					if _, ok := activation.resource.(ActivationResource); !ok {
						err = errors.New("plugin: background setup requires an activation resource")
					}
				}
			}
		}
		activation.setupErr = err
		close(activation.ready)
		stop()
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			return nil, errors.Join(err, activation.close())
		}
		return reply(true, nil, nil)
	})
}

func invokeSetup(handler func(context.Context, Setup) (io.Closer, error), ctx context.Context, setup Setup) (resource io.Closer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin: setup handler panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return handler(ctx, setup)
}

func validateSetup(values []SetupContribution) error {
	seen := map[string]bool{}
	for _, value := range values {
		if !extension.ValidExportName(value.Export) || seen[value.ID] || value.Requires == nil {
			return errors.New("plugin: invalid or duplicate setup contribution")
		}
		if value.Background != nil {
			if err := value.Background.Validate(); err != nil {
				return err
			}
			if value.Background.Scope != "installation" {
				return errors.New("plugin: background role must identify its installation")
			}
			if err := validateSetupInstallation(value.Background.Reference); err != nil {
				return err
			}
		}
		seen[value.ID] = true
		if err := validateSetupID(value.ID); err != nil {
			return err
		}
		dependencies := map[string]bool{}
		for _, dependency := range value.Requires {
			if dependencies[dependency.ID] {
				return errors.New("plugin: duplicate setup dependency")
			}
			dependencies[dependency.ID] = true
			if err := validateSetupBindings(dependency.ID, dependency.Bindings); err != nil {
				return err
			}
			contract, err := extension.AdmitCapability(dependency.Contract)
			if err != nil {
				return err
			}
			if !dependency.Optional && len(dependency.Bindings) == 0 || contract.Descriptor().Composition == extension.Selection && len(dependency.Bindings) > 1 {
				return errors.New("plugin: invalid setup dependency cardinality")
			}
			for _, binding := range dependency.Bindings {
				actual, err := extension.AdmitCapability(binding.Contract)
				if err != nil {
					return err
				}
				if actual.Identity() != contract.Identity() {
					return fmt.Errorf("plugin: dependency %s binding contract mismatch", dependency.ID)
				}
			}
		}
	}
	return nil
}

func SetUp(ctx context.Context, session *sdk.ClientSession, contributions []SetupContribution, metadata sdk.ParamsBase) error {
	if err := validateSetup(contributions); err != nil {
		return err
	}
	result, err := sdk.CallCustomMethod[*setupParams, *methodResult[bool]](ctx, session, setupMethod, &setupParams{ParamsBase: metadata, Protocol: extension.Protocol, Contributions: slices.Clone(contributions)})
	if err != nil {
		return err
	}
	value, err := result.value(setupMethod)
	if err == nil && !value {
		return errors.New("plugin: backend setup was not accepted")
	}
	return err
}

func validateSetupInstallation(value string) error {
	_, err := identity.ParseInstallation(value)
	return err
}
func validateSetupID(value string) error {
	if !identity.ValidDeclarationID(value) {
		return errors.New("plugin: invalid declaration identity")
	}
	return nil
}
func validateSetupBindings(id string, bindings []SetupBinding) error {
	if err := validateSetupID(id); err != nil {
		return err
	}
	if bindings == nil {
		return errors.New("plugin: setup dependency bindings are required")
	}
	for _, value := range bindings {
		if err := validateSetupInstallation(value.InstallationID); err != nil {
			return err
		}
		if _, err := identity.ParseDigest(value.Digest); err != nil {
			return err
		}
		if err := validateSetupID(value.Contribution); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backend) registerActivation() error {
	return sdk.AddReceivingCustomMethod(b.server, activateMethod, func(ctx context.Context, session *sdk.ServerSession, params *protocolParams) (*methodResult[bool], error) {
		if params == nil || params.Protocol != extension.Protocol {
			return nil, errors.New("plugin: current activation protocol is required")
		}
		b.mu.RLock()
		activation := b.activations[session]
		available := b.published && !b.closed
		b.mu.RUnlock()
		if !available || activation == nil {
			return nil, errors.New("plugin: setup is required before activation")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-activation.ready:
		}
		activation.mu.Lock()
		defer activation.mu.Unlock()
		if activation.closed || activation.lifetime.Err() != nil || activation.setupErr != nil {
			return nil, errors.New("plugin: setup lifetime is unavailable")
		}
		if !activation.startAttempted {
			activation.startAttempted = true
			if resource, ok := activation.resource.(ActivationResource); ok {
				activation.startErr = startActivation(resource)
			}
		}
		if activation.startErr != nil {
			return nil, activation.startErr
		}
		return reply(true, nil, nil)
	})
}
func startActivation(resource ActivationResource) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("plugin: activation start panicked: %v\n%s", value, debug.Stack())
		}
	}()
	return resource.Start()
}
func Activate(ctx context.Context, session *sdk.ClientSession) error {
	result, err := sdk.CallCustomMethod[*protocolParams, *methodResult[bool]](ctx, session, activateMethod, &protocolParams{Protocol: extension.Protocol})
	if err != nil {
		return err
	}
	accepted, err := result.value(activateMethod)
	if err == nil && !accepted {
		return errors.New("plugin: backend activation was not accepted")
	}
	return err
}
