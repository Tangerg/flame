package testsupport

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
)

func ScheduleInvocation() portable.Invocation {
	return portable.Invocation{ID: "schedule-trigger", Origin: extension.Automatic, Parent: "parent-trigger", Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "12345678-1234-1234-1234-123456789abc"}}
}
