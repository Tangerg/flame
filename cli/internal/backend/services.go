// Package backend defines the application boundary assembled by a runtime
// adapter. Each use case still consumes its own narrow port; Services only
// gives the process composition root one explicit, discoverable manifest.
package backend

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/agentmemory"
	"github.com/Tangerg/flame/cli/internal/authoringcontext"
	"github.com/Tangerg/flame/cli/internal/changefeed"
	"github.com/Tangerg/flame/cli/internal/diagnostictool"
	"github.com/Tangerg/flame/cli/internal/feedback"
	"github.com/Tangerg/flame/cli/internal/goal"
	"github.com/Tangerg/flame/cli/internal/hookpolicy"
	"github.com/Tangerg/flame/cli/internal/knowledge"
	"github.com/Tangerg/flame/cli/internal/mcp"
	"github.com/Tangerg/flame/cli/internal/modelconfig"
	"github.com/Tangerg/flame/cli/internal/runtimeprofile"
	"github.com/Tangerg/flame/cli/internal/schedule"
	"github.com/Tangerg/flame/cli/internal/sessiontransfer"
	"github.com/Tangerg/flame/cli/internal/skills"
	"github.com/Tangerg/flame/cli/internal/usage"
	"github.com/Tangerg/flame/cli/internal/workspace"
)

// Services is one coherent connection to a backend runtime.
type Services struct {
	Agent            agent.Runtime
	RuntimeProfile   *runtimeprofile.Profile
	Workspaces       workspace.Service
	Changes          changefeed.Source
	Transfers        sessiontransfer.Service
	Usage            usage.Service
	ModelConfig      modelconfig.Service
	Goals            goal.Service
	Skills           skills.Service
	MCP              mcp.Service
	Schedules        schedule.Service
	AgentMemory      agentmemory.Service
	Knowledge        knowledge.Service
	DiagnosticTools  diagnostictool.Service
	AuthoringContext authoringcontext.Service
	Hooks            hookpolicy.Service
	Feedback         feedback.Service
}

// AgentOnly builds the intentionally reduced composition used by the scripted
// demo runtime and focused tests. Auxiliary commands stay visible but explain
// that their service is unavailable.
func AgentOnly(runtime agent.Runtime) Services {
	return Services{Agent: runtime}
}

// Validate checks the minimum contract every CLI mode requires. Auxiliary
// services are optional because a negotiated runtime composition may omit them.
func (s Services) Validate() error {
	if s.Agent == nil {
		return errors.New("backend services: agent runtime is required")
	}
	if s.RuntimeProfile != nil {
		if err := s.RuntimeProfile.Validate(); err != nil {
			return fmt.Errorf("backend services: %w", err)
		}
	}
	return nil
}
