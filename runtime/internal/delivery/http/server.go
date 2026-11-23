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

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/go-chi/chi/v5"
)

const serverReadHeaderTimeout = 10 * time.Second

type Server struct {
	info     RuntimeInfo
	serverID string

	authentication Authentication
	corsOrigins    []string
	healthProbes   []*healthProbeRunner

	rpc           http.Handler
	resources     http.Handler
	pluginControl http.Handler
	web           http.Handler

	httpServer   *http.Server
	handlerCtx   context.Context
	stopHandlers context.CancelFunc

	mu      sync.Mutex
	started bool
}

type Config struct {
	Endpoint *delivery.Endpoint

	PluginControl http.Handler

	Addr string

	ServerInfo protocol.ServerInfo

	ServerID string

	Authentication Authentication

	CORSOrigins []string

	WebApplication http.Handler

	HealthProbes []HealthProbe
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Endpoint == nil || cfg.PluginControl == nil {
		return nil, errors.New("http: Endpoint and PluginControl are required")
	}
	if cfg.Addr == "" {
		return nil, errors.New("http: Addr is required")
	}
	if err := cfg.Authentication.validateListener(cfg.Addr); err != nil {
		return nil, err
	}
	if err := delivery.ValidateInstanceID(cfg.ServerInfo.InstanceID); err != nil {
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
	rpc, err := NewRPCHandler(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	handlerCtx, stopHandlers := context.WithCancel(context.Background())
	s := &Server{
		serverID:       serverID,
		authentication: cfg.Authentication,
		corsOrigins:    slices.Clone(cfg.CORSOrigins),
		healthProbes:   newHealthProbeRunners(cfg.HealthProbes),
		rpc:            rpc,
		resources:      NewPluginResourceHandler(cfg.Endpoint),
		pluginControl:  cfg.PluginControl,
		web:            cfg.WebApplication,
		handlerCtx:     handlerCtx,
		stopHandlers:   stopHandlers,
		info:           newInfoResponse(cfg.ServerInfo),
	}
	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: serverReadHeaderTimeout,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.instrumentRequests, corsMiddleware(s.corsOrigins))

	r.NotFound(s.protectRPC(http.NotFoundHandler()).ServeHTTP)
	r.MethodNotAllowed(s.protectRPC(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods := make(map[string]bool)
		for _, route := range endpointRegistry.Endpoints {
			if !methods[route.Method] && r.Match(chi.NewRouteContext(), route.Method, request.URL.Path) {
				methods[route.Method] = true
				w.Header().Add("Allow", route.Method)
			}
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})).ServeHTTP)
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

// Runtime-owned runs are deliberately not tied to this context: canceling a
// streaming request only detaches its client.
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

func (s *Server) Shutdown(ctx context.Context) error {
	s.stopHandlers()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Close() error {
	s.stopHandlers()
	return s.httpServer.Close()
}
