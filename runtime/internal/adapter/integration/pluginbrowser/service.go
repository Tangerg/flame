package pluginbrowser

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
	controlhost "github.com/Tangerg/flame/qimu/control/host"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type Config struct {
	Bridge              *controlhost.Bridge
	Registry            *plugins.Registry
	ControlEndpoint     string
	MaximumMessageBytes int
	ProofCapacity       int
	PollInterval        time.Duration
}

type Service struct {
	config   Config
	contract qimu.ContractRef
}

func New(config Config) (*Service, error) {
	if config.Bridge == nil || config.Registry == nil || config.MaximumMessageBytes < 1 || config.ProofCapacity < 1 || config.PollInterval < time.Millisecond {
		return nil, errors.New("plugin browser: bridge, registry and explicit transport budgets are required")
	}
	endpoint, err := url.Parse(config.ControlEndpoint)
	if err != nil || endpoint == nil || endpoint.User != nil || endpoint.Fragment != "" || endpoint.RawQuery != "" || endpoint.Path == "" || (endpoint.IsAbs() && endpoint.Scheme != "http" && endpoint.Scheme != "https") || (!endpoint.IsAbs() && (!strings.HasPrefix(endpoint.Path, "/") || endpoint.Host != "")) {
		return nil, errors.New("plugin browser: control endpoint must be an HTTP URL or an absolute same-origin path")
	}
	contract, err := plugins.ComponentContract()
	if err != nil {
		return nil, err
	}
	return &Service{config: config, contract: contract.Ref()}, nil
}

func (s *Service) Open(ctx context.Context, request app.OpenRequest) (app.Session, error) {
	var credentials controlhost.Credentials
	var err error
	if request.Resume != nil {
		credentials, err = s.config.Bridge.ResumeRealm(ctx, *request.Resume, request.ResumeToken)
	} else {
		credentials, err = s.config.Bridge.OpenRealm(ctx, request.OperationID, request.ResumeToken)
	}
	if err != nil {
		return app.Session{}, consumerError(err)
	}
	return app.Session{Authority: credentials.Authority, Realm: credentials.Realm, Credential: credentials.Token, ControlEndpoint: s.config.ControlEndpoint, MaximumMessageBytes: s.config.MaximumMessageBytes, ProofCapacity: s.config.ProofCapacity, PollInterval: s.config.PollInterval, ComponentContract: s.contract}, nil
}

func (s *Service) Acquire(ctx context.Context, request app.ComponentRequest) (control.UseGrant, error) {
	credentials, err := s.config.Bridge.Credentials(request.Realm)
	if err != nil {
		return control.UseGrant{}, consumerError(err)
	}
	plan, err := s.config.Registry.ComponentPlan(ctx, controlhost.RealmID(credentials.Realm), request.Installation, request.Digest, request.Component)
	if err != nil {
		return control.UseGrant{}, consumerError(err)
	}
	grant, err := s.config.Bridge.AcquirePlan(ctx, credentials.Token, request.OperationID, plan, request.ParentUseID)
	return grant, consumerError(err)
}

func (s *Service) Close(ctx context.Context, realm control.Realm) error {
	credentials, err := s.config.Bridge.Credentials(realm)
	if err != nil {
		return consumerError(err)
	}
	return consumerError(s.config.Bridge.SealRealm(ctx, credentials.Token))
}

func consumerError(err error) error {
	switch {
	case errors.Is(err, qimu.ErrInvalid), errors.Is(err, qimu.ErrContractMismatch):
		return errors.Join(plugin.ErrInvalid, err)
	case errors.Is(err, qimu.ErrStaleBinding), errors.Is(err, qimu.ErrFenced), errors.Is(err, qimu.ErrClosed):
		return errors.Join(plugin.ErrStale, err)
	case errors.Is(err, qimu.ErrUnavailable), errors.Is(err, qimu.ErrCapacity):
		return errors.Join(plugin.ErrUnavailable, err)
	default:
		return err
	}
}

var _ app.Service = (*Service)(nil)
