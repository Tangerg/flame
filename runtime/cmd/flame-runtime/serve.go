package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	shutdownTelemetry := configureTelemetry(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownTelemetry(context.WithoutCancel(ctx))) }()

	instance, cfg, paths, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	owned := true
	defer func() {
		if owned {
			err = errors.Join(err, closeRuntimeInstance(instance))
		}
	}()
	srv := cfg.Server
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = runtimehttp.DefaultCORSOrigins()
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or FLAME_SERVER_LISTEN)")
	}
	authentication := runtimehttp.LoopbackOnlyAuthentication()
	var token *localruntime.Token
	if !srv.NoLocalToken {
		tokenPath := srv.LocalTokenPath
		if tokenPath == "" {
			tokenPath = paths.dataDirectory.LocalTokenPath()
		}
		token, err = localruntime.OpenToken(tokenPath)
		if err != nil {
			return err
		}
		authentication, err = runtimehttp.LocalTokenAuthentication(token.Value())
		if err != nil {
			return err
		}
	}
	httpServer, err := buildHTTPServer(instance, srv, authentication)
	if err != nil {
		return err
	}
	owned = false
	return runServer(ctx, errw, httpServer, srv.Listen, token, instance)
}

type runtimeCloser interface {
	Close() error
}

func closeRuntimeInstance(instance runtimeCloser) error {
	if instance == nil {
		return nil
	}
	return instance.Close()
}

func buildHTTPServer(instance *bootstrap.Instance, srv config.Server, authentication runtimehttp.Authentication) (*runtimehttp.Server, error) {
	var web http.Handler
	if srv.WebDirectory != "" {
		assets, err := newWebAssets(srv.WebDirectory)
		if err != nil {
			return nil, err
		}
		web = assets
	}
	info := instance.ServerInfo()
	return runtimehttp.NewServer(runtimehttp.Config{
		Endpoint:       instance.Endpoint(),
		PluginControl:  instance.PluginControl(),
		Addr:           srv.Listen,
		ServerInfo:     info,
		Authentication: authentication,
		CORSOrigins:    srv.CORSOrigins,
		WebApplication: web,
		HealthProbes:   []runtimehttp.HealthProbe{storageHealthProbe(instance.CheckStorage), pluginHealthProbe(instance.CheckPlugins)},
	})
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

type runtimeHTTPServer interface {
	Start() error
	Shutdown(context.Context) error
	Close() error
}

func runServer(ctx context.Context, errw io.Writer, httpServer runtimeHTTPServer, addr string, token *localruntime.Token, instance runtimeCloser) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		_, _ = fmt.Fprintf(errw, "%s http listening on %s\n", runtimeLogPrefix, addr)
		// Whether a route is gated is this composition's fact; reading the label off the
		// declaration would print "token-gated" beside an open endpoint.
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
			_, _ = fmt.Fprintf(errw, "%s   %-4s %-20s %-9s %s%s\n",
				runtimeLogPrefix, endpoint.Method, endpoint.Path, endpoint.Name, gate, detail)
		}
		if token != nil {
			_, _ = fmt.Fprintf(errw, "%s local-token gate active; token at %s\n", runtimeLogPrefix, token.Path())
		} else {
			_, _ = fmt.Fprintln(errw, runtimeLogPrefix+" local-token gate disabled")
		}
		errs <- httpServer.Start()
	}()

	select {
	case serveErr := <-errs:
		return errors.Join(
			closeRuntimeInstance(instance),
			normalizeHTTPServerError(serveErr),
			normalizeHTTPServerError(httpServer.Close()),
		)
	case <-ctx.Done():
		_, _ = fmt.Fprintln(errw, runtimeLogPrefix+" shutdown requested, draining...")
	}

	// The Realm cleanup endpoint must remain reachable while Runtime joins
	// original browser exits. Delivery seals new product work itself.
	runtimeErr := closeRuntimeInstance(instance)
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serverShutdownTimeout)
	defer cancel()
	shutdownErr := httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, normalizeHTTPServerError(httpServer.Close()))
	}
	serveErr := normalizeHTTPServerError(<-errs)
	return errors.Join(runtimeErr, shutdownErr, serveErr)
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
