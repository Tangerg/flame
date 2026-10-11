package plugin

import (
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type ServiceKind string

const (
	MemoryService    ServiceKind = "memory"
	SchedulesService ServiceKind = "schedules"
)

// Service binds one coherent fact owner to an installation-origin MCP process.
// Its private database follows the installation; no host store implements it.
type Service struct {
	Kind   ServiceKind
	Server mcpserver.ServerName
}

func (b *Builder) AdmitService(service Service) error {
	if service.Kind != MemoryService && service.Kind != SchedulesService {
		return refuse(DiagnosticInvalidDeclaration, "unknown managed service %q", service.Kind)
	}
	if slices.ContainsFunc(b.declaration.Services, func(value Service) bool { return value.Kind == service.Kind }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate managed service %q", service.Kind)
	}
	if slices.ContainsFunc(b.declaration.Services, func(value Service) bool { return value.Server == service.Server }) {
		return refuse(DiagnosticInvalidDeclaration, "managed server %q already owns a service kind", service.Server)
	}
	server, found := b.release().server(service.Server)
	if !found || server.Transport != mcpserver.TransportStdio {
		return refuse(DiagnosticInvalidDependencies, "managed service %q requires a declared stdio server", service.Kind)
	}
	b.declaration.Services = append(b.declaration.Services, service)
	return nil
}

func (r Release) Service(kind ServiceKind) (Service, bool) {
	index := slices.IndexFunc(r.declaration.Services, func(value Service) bool { return value.Kind == kind })
	if index < 0 {
		return Service{}, false
	}
	return r.declaration.Services[index], true
}
