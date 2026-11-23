// Package telemetry owns the attribute vocabulary that Runtime logs, spans and metrics share, so a
// fact keeps one key in every signal. OpenTelemetry semantic conventions own the standard keys.
package telemetry

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Re-exporting the standard keys fixes the semantic-conventions version in one place.
const (
	SchemaURL                 = semconv.SchemaURL
	ServiceNameKey            = semconv.ServiceNameKey
	ServiceVersionKey         = semconv.ServiceVersionKey
	SessionIDKey              = semconv.SessionIDKey
	ErrorTypeKey              = semconv.ErrorTypeKey
	RPCMethodKey              = semconv.RPCMethodKey
	HTTPRequestMethodKey      = semconv.HTTPRequestMethodKey
	HTTPRouteKey              = semconv.HTTPRouteKey
	HTTPResponseStatusCodeKey = semconv.HTTPResponseStatusCodeKey
	HTTPResponseBodySizeKey   = semconv.HTTPResponseBodySizeKey
)

const (
	RunIDKey      attribute.Key = "run.id"
	ProviderIDKey attribute.Key = "flame.provider.id"
	ModelKey      attribute.Key = "gen_ai.request.model"
	ProcessIDKey  attribute.Key = "process.id"
	MCPServerKey  attribute.Key = "mcp.server.name"
	// One key names the recall count on spans and the counter that totals it.
	MemoryRecalledKey attribute.Key = "memory.recalled"
)

const (
	runLossKey           = "run.loss"
	goalIncarnationIDKey = "goal.incarnation.id"
	installationIDKey    = "installation.id"
	contributionIDKey    = "contribution.id"
	endpointIDKey        = "endpoint.id"
	callerIDKey          = "caller.id"
	toolNameKey          = "tool.name"
	toolCallIDKey        = "call.id"
	healthProbeKey       = "probe"
	projectionKey        = "projection"
	errorKey             = "error"
	panicKey             = "panic"
)

func Error(err error) slog.Attr { return slog.Any(errorKey, err) }

// Panic must be built in the recovering goroutine: the stack is captured where it is called.
func Panic(recovered any) slog.Attr {
	return slog.Group(panicKey, "value", fmt.Sprint(recovered), "stack", string(debug.Stack()))
}

func SessionID(id identity.SessionID) slog.Attr {
	return slog.String(string(SessionIDKey), id.String())
}

func RunID(id identity.RunID) slog.Attr { return slog.String(string(RunIDKey), id.String()) }

func RunLoss(loss string) slog.Attr { return slog.String(runLossKey, loss) }

func GoalIncarnationID(id string) slog.Attr { return slog.String(goalIncarnationIDKey, id) }

func InstallationID(id identity.InstallationID) slog.Attr {
	return slog.String(installationIDKey, id.String())
}

func ContributionID(id string) slog.Attr { return slog.String(contributionIDKey, id) }

func ProviderID(id string) slog.Attr { return slog.String(string(ProviderIDKey), id) }

func Model(model string) slog.Attr { return slog.String(string(ModelKey), model) }

func EndpointID(id string) slog.Attr { return slog.String(endpointIDKey, id) }

func CallerID(id string) slog.Attr { return slog.String(callerIDKey, id) }

func ToolName(name string) slog.Attr { return slog.String(toolNameKey, name) }

func ToolCallID(id string) slog.Attr { return slog.String(toolCallIDKey, id) }

func ProcessID(id string) slog.Attr { return slog.String(string(ProcessIDKey), id) }

func MCPServer(name string) slog.Attr { return slog.String(string(MCPServerKey), name) }

func HealthProbe(name string) slog.Attr { return slog.String(healthProbeKey, name) }

func RPCMethod(method string) slog.Attr { return slog.String(string(RPCMethodKey), method) }

func HTTPRequestMethod(method string) slog.Attr {
	return slog.String(string(HTTPRequestMethodKey), method)
}

func HTTPRoute(route string) slog.Attr { return slog.String(string(HTTPRouteKey), route) }

func ScopeRequestProjection() slog.Attr { return slog.String(projectionKey, "scope_request") }
