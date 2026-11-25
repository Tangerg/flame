package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
)

const (
	runtimeLogPrefix      = "[flame]"
	serverShutdownTimeout = 10 * time.Second
)

func run(ctx context.Context, errw io.Writer) (err error) {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry := configureTelemetry(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownTelemetry(context.WithoutCancel(ctx))) }()

	instance, cfg, paths, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	httpServer, token, err := configureHTTPServer(instance, cfg.Server, paths.dataDirectory)
	if err != nil {
		return errors.Join(err, instance.Close())
	}
	return serveRuntime(ctx, errw, instance, httpServer, token)
}

func configureHTTPServer(instance *bootstrap.Instance, srv config.Server, data localruntime.DataDirectory) (*runtimehttp.Server, *localruntime.Token, error) {
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = runtimehttp.DefaultCORSOrigins()
	}
	if srv.Listen == "" {
		return nil, nil, errors.New("server.listen is empty (set config server.listen or FLAME_SERVER_LISTEN)")
	}
	authentication := runtimehttp.LoopbackOnlyAuthentication()
	var token *localruntime.Token
	if !srv.NoLocalToken {
		tokenPath := srv.LocalTokenPath
		if tokenPath == "" {
			tokenPath = data.LocalTokenPath()
		}
		var err error
		token, err = localruntime.OpenToken(tokenPath)
		if err != nil {
			return nil, nil, err
		}
		authentication, err = runtimehttp.LocalTokenAuthentication(token.Value())
		if err != nil {
			return nil, nil, err
		}
	}
	var web http.Handler
	if srv.WebDirectory != "" {
		assets, err := newWebAssets(srv.WebDirectory)
		if err != nil {
			return nil, nil, err
		}
		web = assets
	}
	httpServer, err := runtimehttp.NewServer(runtimehttp.Config{
		Endpoint:       instance.Endpoint(),
		PluginControl:  instance.PluginControl(),
		Addr:           srv.Listen,
		ServerInfo:     instance.ServerInfo(),
		Authentication: authentication,
		CORSOrigins:    srv.CORSOrigins,
		WebApplication: web,
		HealthProbes:   []runtimehttp.HealthProbe{storageHealthProbe(instance.CheckStorage), pluginHealthProbe(instance.CheckPlugins)},
	})
	return httpServer, token, err
}

// Storage opens before the server listens, so a probe that loses the race for the
// single pooled connection means busy, not broken.
func storageHealthProbe(check func(context.Context) error) runtimehttp.HealthProbe {
	return runtimehttp.HealthProbe{
		Name: "storage",
		Probe: func(ctx context.Context) runtimehttp.HealthCheck {
			err := check(ctx)
			switch {
			case err == nil:
				return runtimehttp.HealthCheck{Status: runtimehttp.HealthOK}
			case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				return runtimehttp.HealthCheck{
					Status: runtimehttp.HealthDegraded,
					Detail: "storage did not answer within the health budget",
				}
			default:
				return runtimehttp.HealthCheck{
					Status: runtimehttp.HealthUnhealthy,
					Detail: err.Error(),
				}
			}
		},
	}
}

func resolvedVersion() string {
	if version != "" {
		return version
	}
	return bootstrap.BuildVersion()
}

func serveRuntime(ctx context.Context, errw io.Writer, instance *bootstrap.Instance, httpServer *runtimehttp.Server, token *localruntime.Token) error {
	address, stopped, err := httpServer.Start(ctx)
	if err != nil {
		return errors.Join(err, instance.Close(), normalizeHTTPServerError(httpServer.Close()))
	}
	reportListener(errw, address, token)
	select {
	case serveErr := <-stopped:
		return errors.Join(
			instance.Close(),
			normalizeHTTPServerError(serveErr),
			normalizeHTTPServerError(httpServer.Close()),
		)
	case <-ctx.Done():
		_, _ = fmt.Fprintln(errw, runtimeLogPrefix+" shutdown requested, draining...")
	}

	// The Realm cleanup endpoint must remain reachable while Runtime joins
	// original browser exits. Delivery seals new product work itself.
	runtimeErr := instance.Close()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serverShutdownTimeout)
	defer cancel()
	shutdownErr := httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, normalizeHTTPServerError(httpServer.Close()))
	}
	return errors.Join(runtimeErr, shutdownErr, normalizeHTTPServerError(<-stopped))
}

func reportListener(w io.Writer, address net.Addr, token *localruntime.Token) {
	_, _ = fmt.Fprintf(w, "%s http listening on %s\n", runtimeLogPrefix, address)
	for _, endpoint := range runtimehttp.Contract().Endpoints {
		gate := "token-gated"
		switch {
		case endpoint.Authentication == runtimehttp.EndpointAuthenticationNone:
			gate = "no auth"
		case endpoint.Authentication == runtimehttp.EndpointAuthenticationRealm:
			gate = "realm-gated"
		case token == nil:
			gate = "UNGATED"
		}
		detail := ""
		if endpoint.Kind == runtimehttp.EndpointKindRPC {
			detail = "  (streaming methods -> text/event-stream)"
		}
		_, _ = fmt.Fprintf(w, "%s   %-4s %-20s %-9s %s%s\n", runtimeLogPrefix, endpoint.Method, endpoint.Path, endpoint.Name, gate, detail)
	}
	if token == nil {
		_, _ = fmt.Fprintln(w, runtimeLogPrefix+" local-token gate disabled")
		return
	}
	_, _ = fmt.Fprintf(w, "%s local-token gate active; token at %s\n", runtimeLogPrefix, token.Path())
}

func normalizeHTTPServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func pluginHealthProbe(check func(context.Context) error) runtimehttp.HealthProbe {
	return runtimehttp.HealthProbe{Name: "plugins", Probe: func(ctx context.Context) runtimehttp.HealthCheck {
		if err := check(ctx); err != nil {
			return runtimehttp.HealthCheck{Status: runtimehttp.HealthDegraded, Detail: "original plugin execution protection is awaiting recovery"}
		}
		return runtimehttp.HealthCheck{Status: runtimehttp.HealthOK}
	}}
}
