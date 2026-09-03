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

	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/internal/infra/telemetry"
	"github.com/Tangerg/flame/runtime/localruntime"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	runtimeLogPrefix      = "[flame]"
	serverShutdownTimeout = 10 * time.Second
	runtimeCloseAttempts  = 3
)

func run(ctx context.Context, errw io.Writer) (err error) {
	shutdownTelemetry := telemetry.Configure(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownTelemetry(context.WithoutCancel(ctx))) }()

	instance, cfg, paths, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeRuntimeInstance(instance)) }()
	srv := cfg.Server
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = flamehttp.DefaultCORSOrigins()
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or FLAME_SERVER_LISTEN)")
	}
	var token *localruntime.Token
	if !srv.NoLocalToken {
		tokenPath := srv.LocalTokenPath
		if tokenPath == "" {
			tokenPath = paths.dataDirectory.LocalTokenPath()
		}
		t, openTokenErr := localruntime.OpenToken(tokenPath)
		if openTokenErr != nil {
			return openTokenErr
		}
		token = t
	}

	tokenValue := ""
	if token != nil {
		tokenValue = token.Value()
	}
	httpServer, err := buildHTTPServer(instance, srv, tokenValue)
	if err != nil {
		return err
	}
	return runServer(ctx, errw, httpServer, srv.Listen, token)
}

type runtimeCloser interface {
	Close() error
}

func closeRuntimeInstance(instance runtimeCloser) error {
	if instance == nil {
		return nil
	}
	errorsByAttempt := make([]error, 0, runtimeCloseAttempts)
	for range runtimeCloseAttempts {
		err := instance.Close()
		if err == nil {
			return nil
		}
		errorsByAttempt = append(errorsByAttempt, err)
	}
	return fmt.Errorf(
		"close runtime after %d attempts: %w",
		runtimeCloseAttempts,
		errors.Join(errorsByAttempt...),
	)
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved settings.
func buildHTTPServer(instance *bootstrap.Instance, srv config.Server, tokenValue string) (*flamehttp.Server, error) {
	info := instance.ServerInfo()
	return flamehttp.NewServer(flamehttp.Config{
		Endpoint:        instance.Endpoint(),
		Addr:            srv.Listen,
		ServerInfo:      info,
		ProtocolVersion: protocol.ProtocolVersion,
		LocalToken:      tokenValue,
		CORSOrigins:     srv.CORSOrigins,
	})
}

// resolvedVersion keeps HTTP identity and telemetry resource metadata aligned:
// an explicit link-time version wins, then Go module build info, then "dev".
func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	return flamehttp.ServerInfoOrDefault().Version
}

type runtimeHTTPServer interface {
	Start() error
	Shutdown(context.Context) error
	Close() error
}

// runServer launches the server, blocks until it returns or a shutdown signal
// arrives, then drains with a 10s budget.
func runServer(ctx context.Context, errw io.Writer, httpServer runtimeHTTPServer, addr string, token *localruntime.Token) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		_, _ = fmt.Fprintf(errw, "%s http listening on %s\n", runtimeLogPrefix, addr)
		_, _ = fmt.Fprintf(errw, "%s   POST /v2/rpc              JSON-RPC (streaming methods -> text/event-stream)\n", runtimeLogPrefix)
		_, _ = fmt.Fprintf(errw, "%s   GET  /v2/info             metadata (no auth)\n", runtimeLogPrefix)
		_, _ = fmt.Fprintf(errw, "%s   GET  /v2/health/live      liveness\n", runtimeLogPrefix)
		_, _ = fmt.Fprintf(errw, "%s   GET  /v2/health/ready     dependency readiness\n", runtimeLogPrefix)
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
			normalizeHTTPServerError(serveErr),
			normalizeHTTPServerError(httpServer.Close()),
		)
	case <-ctx.Done():
		_, _ = fmt.Fprintln(errw, runtimeLogPrefix+" shutdown requested, draining...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serverShutdownTimeout)
	defer cancel()
	shutdownErr := httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, normalizeHTTPServerError(httpServer.Close()))
	}
	serveErr := normalizeHTTPServerError(<-errs)
	return errors.Join(shutdownErr, serveErr)
}

func normalizeHTTPServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
