package mcp

import (
	"context"
	"errors"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

// An anonymous probe of an OAuth-protected server would always 401 even though the
// live connection is authorized, so the probe reuses the session's sign-in.
func (c *connections) probe(ctx context.Context, cfg ServerConfig) error {
	if cfg.OAuthHandler == nil && c != nil {
		c.mu.Lock()
		if configuredServer := c.find(cfg.ID()); configuredServer != nil {
			cfg.OAuthHandler = reusableOAuth(configuredServer.config, cfg, configuredServer.oauth)
		}
		c.mu.Unlock()
	}
	return probe(ctx, cfg)
}

func probe(ctx context.Context, cfg ServerConfig) (err error) {
	if _, installed := cfg.Source.Origin().Installation(); installed {
		return errors.New("mcp: probe requires a user server candidate")
	}
	if validateErr := cfg.Validate(); validateErr != nil {
		return validateErr
	}
	cfg = cfg.Clone()
	ctx, span := tracer.Start(ctx, "mcp.probe",
		trace.WithAttributes(telemetry.MCPServerKey.String(cfg.ID().String())))
	defer span.End()

	client := sdk.NewClient(clientImplementation(), nil)
	session, cleanupSession, err := dial(ctx, ctx, client, &launch{config: cfg})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer func() {
		if closeErr := errors.Join(session.Close(), cleanupSession()); closeErr != nil {
			span.RecordError(closeErr)
			err = errors.Join(err, closeErr)
		}
	}()
	if _, err := sourceTools(ctx, nil, cfg, session); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
