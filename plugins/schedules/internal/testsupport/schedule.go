package testsupport

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"

	framecall "github.com/Tangerg/frame/call"
)

func ScheduleInvocation() framecall.Invocation {
	return framecall.Invocation{ID: "schedule-trigger", Origin: extension.Automatic, Parent: "parent-trigger", Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: "12345678-1234-1234-1234-123456789abc"}}
}
