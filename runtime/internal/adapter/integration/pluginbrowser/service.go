package pluginbrowser

import (
	framebrowser "github.com/Tangerg/frame/browser"

	"context"
	"errors"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

type Config struct {
	Bridge              *framebrowser.Bridge
	Registry            *plugins.Registry
	ControlEndpoint     string
	ProofCapacity       int
	PreparationCapacity int
	PollInterval        time.Duration
}

type Service struct {
	config   Config
	contract frame.ContractRef
}

func New(config Config) (*Service, error) {
	if config.Bridge == nil || config.Registry == nil || config.ProofCapacity < 1 || config.PreparationCapacity < 1 || config.PollInterval < time.Millisecond {
		return nil, errors.New("plugin browser: bridge, registry and explicit transport budgets are required")
	}
	if !validControlEndpoint(config.ControlEndpoint) {
		return nil, errors.New("plugin browser: control endpoint must be an HTTP URL or an absolute same-origin path")
	}
	contract, err := plugins.ComponentContract()
	if err != nil {
		return nil, err
	}
	return &Service{config: config, contract: contract.Ref()}, nil
}

// The browser reaches the control service either at an absolute HTTP(S)
// endpoint or at a path on the page's own origin.
func validControlEndpoint(raw string) bool {
	if strings.ContainsAny(raw, "?#") {
		return false
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		_, err := url.Parse(raw)
		return err == nil
	}
	endpoint, err := httporigin.ParseEndpoint(raw)
	return err == nil && endpoint.URL().Path != ""
}

func (s *Service) Open(ctx context.Context, request app.OpenRequest) (app.Session, error) {
	var credentials framebrowser.Credentials
	var err error
	if request.Resume != nil {
		credentials, err = s.config.Bridge.ResumeRealm(ctx, *request.Resume, request.ResumeToken)
	} else {
		credentials, err = s.config.Bridge.OpenRealm(ctx, request.OperationID, request.ResumeToken)
	}
	if err != nil {
		return app.Session{}, consumerError(err)
	}
	if credentials.MaxMessageBytes < 1 || credentials.MaxMessageBytes > math.MaxInt {
		return app.Session{}, frame.ErrInvalid
	}
	return app.Session{Authority: credentials.Authority, Realm: credentials.Realm, Credential: credentials.Token, ControlEndpoint: s.config.ControlEndpoint, MaximumMessageBytes: int(credentials.MaxMessageBytes), ProofCapacity: s.config.ProofCapacity, PreparationCapacity: s.config.PreparationCapacity, PollInterval: s.config.PollInterval, ComponentContract: s.contract}, nil
}

func (s *Service) Acquire(ctx context.Context, request app.ComponentRequest) (framebrowser.UseGrant, error) {
	credentials, err := s.config.Bridge.Credentials(request.Realm)
	if err != nil {
		return framebrowser.UseGrant{}, consumerError(err)
	}
	plan, err := s.config.Registry.ComponentPlan(ctx, framebrowser.RealmID(credentials.Realm), request.Installation, request.Digest, request.Component)
	if err != nil {
		return framebrowser.UseGrant{}, consumerError(err)
	}
	grant, err := s.config.Bridge.AcquirePlan(ctx, credentials.Token, request.OperationID, plan, request.ParentUseID)
	return grant, consumerError(err)
}

func (s *Service) Close(ctx context.Context, realm framebrowser.Realm) error {
	credentials, err := s.config.Bridge.Credentials(realm)
	if err != nil {
		return consumerError(err)
	}
	return consumerError(s.config.Bridge.SealRealm(ctx, credentials.Token))
}

func consumerError(err error) error {
	switch {
	case errors.Is(err, frame.ErrInvalid), errors.Is(err, frame.ErrContractMismatch):
		return errors.Join(plugin.ErrInvalid, err)
	case errors.Is(err, frame.ErrStaleBinding), errors.Is(err, frame.ErrFenced), errors.Is(err, frame.ErrClosed):
		return errors.Join(plugin.ErrStale, err)
	case errors.Is(err, frame.ErrUnavailable), errors.Is(err, frame.ErrCapacity):
		return errors.Join(plugin.ErrUnavailable, err)
	default:
		return err
	}
}

var _ app.Service = (*Service)(nil)
