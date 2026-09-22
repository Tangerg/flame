package delivery

import (
	"errors"
	"strings"
	"testing"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestOperationProblemsSeparateTypeDetailAndCause(t *testing.T) {
	for _, tc := range []struct {
		name             string
		err, kind, cause error
	}{
		{"bare resource failure", protocol.ErrSessionNotFound, protocol.ErrSessionNotFound, protocol.ErrSessionNotFound},
		{"workspace", wireWorkspaceError(workspaceapp.ErrCWDUnavailable), protocol.ErrWorkspaceUnavailable, workspaceapp.ErrCWDUnavailable},
		{"mcp", wireMCPError(mcpapp.ErrUnknownServer), protocol.ErrMCPServerNotFound, mcpapp.ErrUnknownServer},
		{"schedule revision", mapScheduleErr(schedule.ErrRevisionConflict, "sch_1"), protocol.ErrRevisionConflict, schedule.ErrRevisionConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := ProjectError(tc.err)
			problem := failure.Problem()
			if problem.Type != tc.kind.Error() || strings.HasPrefix(problem.Detail, problem.Type) {
				t.Fatalf("machine type leaked into human detail: %+v", problem)
			}
			if !errors.Is(failure, tc.kind) || !errors.Is(failure, tc.cause) {
				t.Fatalf("lost error identity: %v", failure)
			}
		})
	}
}

func TestExplicitFailuresPreserveStructuredDetails(t *testing.T) {
	constraint := &protocol.ConstraintError{Shape: "RunInput", Fields: []protocol.FieldError{{Field: "input[0].text", Detail: "must not be empty"}}}
	failure := ProjectError(NewFailure(errors.Join(protocol.ErrInvalidParams, constraint), "invalid run input"))
	problem := failure.Problem()
	if problem.Type != protocol.ErrInvalidParams.Error() || len(problem.Errors) != 1 || problem.Errors[0] != constraint.Fields[0] || !errors.Is(failure, constraint) {
		t.Fatalf("lost field error or cause: %+v", problem)
	}
	constraint.Fields[0].Detail = "changed"
	problem.Errors[0].Detail = "changed again"
	if failure.Problem().Errors[0].Detail != "must not be empty" {
		t.Fatal("projected failure retained mutable field details")
	}
	gap := NewCapabilityGapError(protocol.CapabilityRequirement{Type: protocol.RequirementFeature, Name: "skills"})
	capability := ProjectError(NewFailure(gap, "declare the skills capability")).Problem()
	if capability.Type != protocol.ErrCapabilityNotNeg.Error() || len(capability.RequiredCapabilities) != 1 {
		t.Fatalf("lost capability detail: %+v", capability)
	}
}

func TestProjectErrorRejectsMalformedExplicitFailure(t *testing.T) {
	failure := ProjectError(NewFailure(protocol.ErrSessionHasActiveRun, "missing active run"))
	if !errors.Is(failure, protocol.ErrInternalError) {
		t.Fatalf("invalid structured failure escaped projection: %+v", failure.Problem())
	}
}
