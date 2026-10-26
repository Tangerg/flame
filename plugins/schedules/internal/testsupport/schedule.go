package testsupport

import "github.com/Tangerg/flame/packages/go-sdk/extension"

func ScheduleInvocation() extension.Invocation {
	return extension.Invocation{ID: "schedule-trigger", Origin: extension.Automatic, Parent: "parent-trigger", Target: extension.Target{Scope: extension.ScopeInstallation, Reference: "12345678-1234-1234-1234-123456789abc"}}
}
