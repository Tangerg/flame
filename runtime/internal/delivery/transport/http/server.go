package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

const serverReadHeaderTimeout = 10 * time.Second

type messageDispatcher interface {
	Dispatch(ctx context.Context, message transport.Message) dispatch.Result
}

type Server struct {
	info     RuntimeInfo
	serverID string

	localToken   string
	corsOrigins  []string
	healthProbes []*healthProbeRunner

	router messageDispatcher
	web    http.Handler

	httpServer   *http.Server
	handlerCtx   context.Context
	stopHandlers context.CancelFunc

	mu      sync.Mutex
	started bool
}

type Config struct {
	// Endpoint is the Runtime instance's binding-neutral operation entrypoint.
	// Required. HTTP never constructs a second policy pipeline.
	Endpoint *delivery.Endpoint

	// Addr is the listen address (":8080", "127.0.0.1:0", ...). Required.
	Addr string

	ServerInfo protocol.ServerInfo

	// ServerID identifies this process in X-Server response
	// header. Defaults to ServerInfo.Name + "/" + ServerInfo.Version.
	ServerID string

	// LocalToken, when non-empty, gates POST /v2/rpc with
	// `Authorization: Bearer <LocalToken>` — streaming POSTs included
	// (no header-less EventSource to exempt under streamable HTTP). Only
	// the sidecars bypass. Empty disables the gate — tests + same-origin
	// TUI scenarios.
	LocalToken string

	// CORSOrigins is the exact-match origin allowlist; "*" is honored
	// (without credentials). Empty disables CORS — same-origin only.
	CORSOrigins []string

	// WebApplication handles public browser assets outside the protocol namespace.
	// Bootstrap supplies its confined filesystem adapter; protocol authentication
	// remains owned by this server. Nil disables browser application serving.
	WebApplication http.Handler

	// HealthProbes are the labeled readiness checks invoked on every
	// GET /v2/health/ready. Empty list ⇒ the endpoint always returns ready.
	// Probes run in parallel under a shared 2s budget.
	HealthProbes []HealthProbe
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Endpoint == nil {
		return nil, errors.New("http: Endpoint is required")
	}
	if cfg.Addr == "" {
		return nil, errors.New("http: Addr is required")
	}
	if _, err := runtimeidentity.ParseRuntimeInstance(cfg.ServerInfo.InstanceID); err != nil {
		return nil, fmt.Errorf("http: ServerInfo.InstanceID: %w", err)
	}
	seenProbes := make(map[string]struct{}, len(cfg.HealthProbes))
	for _, probe := range cfg.HealthProbes {
		if probe.Name == "" {
			return nil, errors.New("http: health probe name is required")
		}
		if probe.Probe == nil {
			return nil, errors.New("http: health probe " + probe.Name + " has no function")
		}
		if _, exists := seenProbes[probe.Name]; exists {
			return nil, errors.New("http: duplicate health probe name: " + probe.Name)
		}
		seenProbes[probe.Name] = struct{}{}
	}
	serverID := cfg.ServerID
	if serverID == "" {
		serverID = cfg.ServerInfo.Name + "/" + cfg.ServerInfo.Version
	}
	router, err := dispatch.New(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	handlerCtx, stopHandlers := context.WithCancel(context.Background())
	s := &Server{
		serverID:     serverID,
		localToken:   cfg.LocalToken,
		corsOrigins:  slices.Clone(cfg.CORSOrigins),
		healthProbes: newHealthProbeRunners(cfg.HealthProbes),
		router:       router,
		web:          cfg.WebApplication,
		handlerCtx:   handlerCtx,
		stopHandlers: stopHandlers,
		info:         newInfoResponse(cfg.ServerInfo),
	}
	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: serverReadHeaderTimeout,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	return s, nil
}

// Handler returns the routed handler — exposed so tests can drive it
// with httptest.NewServer without going through Start. Each call
// builds a fresh mux so concurrent tests don't share state.
//
// Middleware order (outer → inner):
//
//	server lifecycle → request instrumentation → cors → authGate → mux
//
// The lifecycle wrapper owns transport cancellation. Request instrumentation remains
// outside the protocol middleware so every request (including CORS preflight
// and 401) is observed; cors precedes authGate so OPTIONS preflights resolve
// without a token; authGate precedes the mux so unauthenticated requests never
// touch handlers.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.instrumentRequests, corsMiddleware(s.corsOrigins), s.authGate)

	// The Delivery registry owns both metadata and handler binding. Sidecars stay
	// flat JSON; the RPC entrypoint is the sole owner of envelope method identity.
	registerEndpoints(r, s)

	if s.web == nil {
		return s.withServerLifecycle(r)
	}
	web := s.instrumentRequests(s.web)
	return s.withServerLifecycle(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == endpointPrefix || strings.HasPrefix(request.URL.Path, endpointPrefix+"/") {
			r.ServeHTTP(w, request)
			return
		}
		web.ServeHTTP(w, request)
	}))
}

// withServerLifecycle cancels transport-owned request work when this server is
// shutting down. Runtime-owned runs are deliberately not tied to this context:
// canceling a streaming request only detaches that client from the run.
func (s *Server) withServerLifecycle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(s.handlerCtx, cancel)
		defer func() {
			stop()
			cancel()
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Start binds the listen address and serves until Shutdown is called.
// Returns http.ErrServerClosed on clean shutdown.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("http: server already started")
	}
	s.started = true
	srv := s.httpServer
	s.mu.Unlock()
	return srv.ListenAndServe()
}

// Shutdown stops transport-owned request work, then gracefully drains the
// server. It is safe before Start and prevents a later Start from binding.
// Runtime-owned runs continue independently after their clients detach.
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopHandlers()
	return s.httpServer.Shutdown(ctx)
}

// Close force-closes listeners and active connections. The process owner uses
// it only when graceful Shutdown exhausts its deadline, to cancel active
// request contexts before application resources are released.
func (s *Server) Close() error {
	s.stopHandlers()
	return s.httpServer.Close()
}
