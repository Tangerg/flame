package http

import (
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/go-chi/chi/v5"
)

// Request headers a client may send to this transport. The reader and the CORS
// allowlist below take the same names: a header read by a handler but missing
// from the allowlist fails a browser client's preflight, and the failure is a
// rejected request rather than anything the handler can report.
const (
	headerAuthorization        = "Authorization"
	headerContentType          = "Content-Type"
	headerIdempotencyKey       = "Idempotency-Key"
	headerIdempotencyNamespace = "Idempotency-Namespace"
	headerLastEventID          = "Last-Event-Id"
	headerRequestID            = "Request-Id"
	headerServer               = "X-Server"
	headerMethod               = "X-Method"
)

// requestHeaders is every header this transport reads, in one list so the
// allowlist cannot fall behind the readers.
func requestHeaders() []string {
	return []string{
		headerAuthorization,
		headerContentType,
		headerIdempotencyKey,
		headerIdempotencyNamespace,
		headerLastEventID,
		headerMethod,
	}
}

// EndpointKind separates the JSON-RPC binding from typed operational sidecars.
// It is a transport fact: neither Application nor the protocol method registry
// needs to know which HTTP path carries an operation.
type EndpointKind string

const (
	EndpointKindRPC      EndpointKind = "rpc"
	EndpointKindSidecar  EndpointKind = "sidecar"
	endpointKindResource EndpointKind = "resource"
	endpointKindControl  EndpointKind = "pluginControl"
)

// EndpointAuthentication states the HTTP-layer gate applied before a handler.
type EndpointAuthentication string

const (
	EndpointAuthenticationNone          EndpointAuthentication = "none"
	EndpointAuthenticationLocalToken    EndpointAuthentication = "localToken"
	EndpointAuthenticationResourceLease EndpointAuthentication = "resourceLease"
	EndpointAuthenticationRealm         EndpointAuthentication = "pluginRealm"
)

const (
	endpointPrefix          = "/v2"
	endpointRPC             = "rpc"
	endpointInfo            = "info"
	endpointLiveness        = "liveness"
	endpointReadiness       = "readiness"
	endpointPluginResources = "pluginResources"
)

const pluginResourcePrefix = endpointPrefix + "/plugin-resources/"

const PluginControlPath = endpointPrefix + "/frame"

// EndpointSpec is one implemented HTTP entrypoint. ResponseType is nil for the
// JSON-RPC binding because its typed method results live in operation.Contract;
// sidecars name their exact flat-JSON response type here.
type EndpointSpec struct {
	Name             string
	Kind             EndpointKind
	Method           string
	Path             string
	Authentication   EndpointAuthentication
	ResponseStatuses []int
	ResponseType     reflect.Type
	Projects         delivery.Name
}

type endpointRegistration struct {
	EndpointSpec
	handler func(*Server, http.ResponseWriter, *http.Request)
}

// EnumSpec supplies the closed string sets used only by sidecar response DTOs.
// The JSON-RPC registry owns its own enums through contractcatalog.
type EnumSpec struct {
	Type   reflect.Type
	Values []string
}

// Clients derive header names from this contract so transport and browser
// preflight rules cannot evolve under separate owners.
type HeaderSpec struct {
	Authorization        string
	ContentType          string
	IdempotencyKey       string
	IdempotencyNamespace string
	LastEventID          string
	RequestID            string
	Server               string
	Method               string
}

// Routing and generated clients consume the same delivery-owned declaration.
type ContractSpec struct {
	Headers   HeaderSpec
	Endpoints []EndpointSpec
	Enums     []EnumSpec
}

var endpointRegistry = struct {
	Endpoints []endpointRegistration
	Enums     []EnumSpec
}{
	Endpoints: []endpointRegistration{
		{
			EndpointSpec: EndpointSpec{Name: "pluginControl", Kind: endpointKindControl, Method: http.MethodPost, Path: PluginControlPath, Authentication: EndpointAuthenticationRealm, ResponseStatuses: []int{http.StatusOK, http.StatusUnauthorized, http.StatusConflict, http.StatusServiceUnavailable}},
			handler: func(server *Server, w http.ResponseWriter, r *http.Request) {
				if server.pluginControl == nil {
					http.Error(w, "plugin control is unavailable", http.StatusServiceUnavailable)
					return
				}
				server.pluginControl.ServeHTTP(w, r)
			},
		},
		{
			EndpointSpec: EndpointSpec{Name: endpointPluginResources, Projects: delivery.PluginsReadResource, Kind: endpointKindResource, Method: http.MethodGet, Path: pluginResourcePrefix + "{lease}/*", Authentication: EndpointAuthenticationResourceLease, ResponseStatuses: []int{http.StatusOK, http.StatusGone, http.StatusInternalServerError}},
			handler:      func(server *Server, w http.ResponseWriter, r *http.Request) { server.resources.ServeHTTP(w, r) },
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointRPC,
				Kind:             EndpointKindRPC,
				Method:           http.MethodPost,
				Path:             endpointPrefix + "/rpc",
				Authentication:   EndpointAuthenticationLocalToken,
				ResponseStatuses: []int{protocol.RPCCallStatus, http.StatusAccepted, http.StatusNoContent},
			},
			handler: func(server *Server, w http.ResponseWriter, r *http.Request) { server.rpc.ServeHTTP(w, r) },
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointInfo,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             endpointPrefix + "/info",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK},
				ResponseType:     reflect.TypeFor[RuntimeInfo](),
			},
			handler: (*Server).handleInfo,
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointLiveness,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             endpointPrefix + "/health/live",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK},
				ResponseType:     reflect.TypeFor[LivenessStatus](),
			},
			handler: (*Server).handleLiveness,
		},
		{
			EndpointSpec: EndpointSpec{
				Name:             endpointReadiness,
				Kind:             EndpointKindSidecar,
				Method:           http.MethodGet,
				Path:             endpointPrefix + "/health/ready",
				Authentication:   EndpointAuthenticationNone,
				ResponseStatuses: []int{http.StatusOK, http.StatusServiceUnavailable},
				ResponseType:     reflect.TypeFor[ReadinessStatus](),
			},
			handler: (*Server).handleReadiness,
		},
	},
	Enums: []EnumSpec{
		{Type: reflect.TypeFor[HealthStatus](), Values: []string{string(HealthOK), string(HealthDegraded), string(HealthUnhealthy)}},
		{Type: reflect.TypeFor[LivenessState](), Values: []string{string(livenessOK)}},
		{Type: reflect.TypeFor[TransportKind](), Values: []string{string(transport)}},
	},
}

// Contract returns an isolated snapshot so generators and tests cannot mutate
// the process's routing authority.
func Contract() ContractSpec {
	contract := ContractSpec{
		Headers: HeaderSpec{
			Authorization: headerAuthorization, ContentType: headerContentType,
			IdempotencyKey: headerIdempotencyKey, IdempotencyNamespace: headerIdempotencyNamespace,
			LastEventID: headerLastEventID, RequestID: headerRequestID,
			Server: headerServer, Method: headerMethod,
		},
		Endpoints: make([]EndpointSpec, len(endpointRegistry.Endpoints)),
		Enums:     make([]EnumSpec, len(endpointRegistry.Enums)),
	}
	for index, registration := range endpointRegistry.Endpoints {
		endpoint := registration.EndpointSpec
		endpoint.ResponseStatuses = slices.Clone(endpoint.ResponseStatuses)
		contract.Endpoints[index] = endpoint
	}
	for index, enum := range endpointRegistry.Enums {
		enum.Values = slices.Clone(enum.Values)
		contract.Enums[index] = enum
	}
	return contract
}

func endpointNamed(name string) EndpointSpec {
	for _, registration := range endpointRegistry.Endpoints {
		if registration.Name == name {
			return registration.EndpointSpec
		}
	}
	panic("http: unknown endpoint " + name)
}

func endpointPath(name string) string {
	return endpointNamed(name).Path
}

// RegisterCallbackEndpoints binds only the Runtime operations Realm callbacks
// are authorized to reach. The enclosing Realm transport authenticates actors.
func RegisterCallbackEndpoints(mux *http.ServeMux, rpc, resources http.Handler) {
	for _, route := range endpointRegistry.Endpoints {
		switch route.Kind {
		case EndpointKindRPC:
			mux.Handle(route.Method+" "+route.Path, rpc)
		case endpointKindResource:
			mux.Handle(route.Method+" "+strings.Replace(route.Path, "/*", "/{resource...}", 1), resources)
		}
	}
}

func registerEndpoints(router *chi.Mux, server *Server) {
	for _, endpoint := range endpointRegistry.Endpoints {
		handler := endpoint.handler
		var routed http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(server, w, r)
		})
		switch endpoint.Authentication {
		case EndpointAuthenticationLocalToken:
			routed = server.protectRPC(routed)
		case EndpointAuthenticationNone, EndpointAuthenticationResourceLease, EndpointAuthenticationRealm:
			// Lease and Realm authentication belong to their endpoint handlers.
		default:
			panic("http: endpoint has invalid authentication policy")
		}
		router.Method(endpoint.Method, endpoint.Path, routed)
	}
}
