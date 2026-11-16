package testsupport

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"

	framemcp "github.com/Tangerg/frame/process/mcp"
)

func ScheduleInvocation() framemcp.Invocation {
	return framemcp.Invocation{ID: "schedule-trigger", Origin: extension.Automatic, Parent: "parent-trigger", Target: framemcp.Target{Scope: extension.ScopeInstallation, Reference: "12345678-1234-1234-1234-123456789abc"}}
}
