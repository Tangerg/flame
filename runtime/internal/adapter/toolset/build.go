package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	scopea2a "github.com/Tangerg/scope/a2a"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the local shell and A2A adapters the tools wrap and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built]. Tool capability
// construction therefore stays outside Agent
// execution (doc/ARCHITECTURE.md).

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	// Lifetime is the process-owned root for local capability resources that
	// outlive the startup call and individual Tool invocations.
	Lifetime         context.Context
	UserHome         string
	PackageSkills    promptsource.PackageSkills
	SkillsUserDir    string
	CodeIntelligence CodeIntelligence
	A2AAgents        []A2AAgentConfig
	Plan             builtin.PlanUseCases   // backs set_plan + exit_plan_mode; nil → both are omitted
	PlanMode         builtin.PlanModePolicy // session-scoped Plan mode; nil → enter/exit are omitted
	Interrupt        runs.InterruptFunc
	ToolResults      builtin.ToolResultStore        // backs read_tool_result (reads offloaded tool output); nil → omitted
	SkillUsage       builtin.SkillUsageRecorder     // records skill loads for the idle-lifecycle curator; nil → use recording off
	SkillProposals   builtin.SkillProposalSubmitter // backs root-only propose_skill; nil → omitted
	GoalReader       goalReader                     // backs get_goal; nil → omitted
	GoalReporter     builtin.GoalOutcomeReporter    // backs report_goal_outcome; nil → omitted

	// MCPToolDisabled reports whether an identified MCP tool is hidden. The
	// runtime updates the underlying policy after every registry change.
	MCPToolDisabled func(tool.Ref) bool
	RestoreMCP      func(context.Context, []tool.Ref, []plugin.Activation) ([]toolcontract.Tool, func() error, error)

	// SandboxShell opts the shell tools into per-command OS isolation: each
	// command runs in an in-place jail rooted at its own cwd (workspace-write
	// only, network denied, $HOME hidden, env scrubbed). Off by default; on a
	// host with no isolation backend enabling it fails the build (fail-closed).
	SandboxShell bool
	// SandboxReadOnlyPaths re-opens declared toolchain roots below the hidden
	// home for reads (e.g. a language toolchain or dependency cache under $HOME)
	// in every jail, including an isolated session's.
	SandboxReadOnlyPaths []string
}

// GoalReader supplies the Goal read model used by get_goal. Outcome-tool
// availability is derived from immutable Run admission provenance instead of
// mutable Goal state.
type goalReader interface {
	builtin.GoalReader
}

// Built is the assembled tool environment: the runtime-scope resolver (also the
// diagnostic tool catalog) and its capability closers.
type Built struct {
	Resolver *Resolver
	// Shells is the background-shell set the shell tools run over. Exposed so
	// live-state reporting can inspect a session's still-running jobs (e.g. a
	// post-compaction live-state reminder) without owning a second shell set.
	Shells  *exec.Shells
	Closers []func() error
}

// Build constructs every capability adapter, assembles the resolver, and
// returns the [Built] environment. Remote MCP connections are constructed by
// their dedicated adapter before this function and supplied as an initial tool
// snapshot; this package owns only the local and A2A capability lifecycle.
func Build(ctx context.Context, config BuildConfig) (_ Built, err error) {
	if ctx == nil {
		return Built{}, errors.New("toolset: startup context is required")
	}
	if config.Lifetime == nil {
		return Built{}, errors.New("toolset: lifetime is required")
	}
	if config.UserHome == "" {
		return Built{}, errors.New("toolset: user home is required")
	}
	if !filepath.IsAbs(config.UserHome) {
		return Built{}, errors.New("toolset: user home must be absolute")
	}
	tracker := newReadTracker()

	// Isolated sessions jail their shell even when the global sandbox.shell
	// opt-in is off, so a confiner failure refuses assembly only under that
	// opt-in; otherwise the shell set keeps the cause for isolated commands.
	confiner, confErr := sandbox.NewConfiner(config.UserHome, config.SandboxReadOnlyPaths)
	shells, err := exec.NewShells(confiner, confErr, config.SandboxShell)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shells: %w", err)
	}
	shellTools, err := builtin.BuildShell(shells)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shell tools: %w", err)
	}
	var a2aTools *scopea2a.ToolSet
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			err = errors.Join(err, a2aTools.Close(), shells.KillAll())
		}
	}()

	interrupt := config.Interrupt
	if interrupt == nil {
		interrupt = runs.InterruptUnavailable
	}

	// ask_user is a build-time Tool shared by root and delegated roles. A child
	// question waits at the same durable tree boundary as child approval.
	askUserTool, err := builtin.NewAskUser(interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build ask_user: %w", err)
	}

	// Plan mode and Plan state form one lifecycle. The family keeps
	// exit_plan_mode bound to the exact store maintained by set_plan.
	planFamily, err := builtin.BuildPlan(config.PlanMode, config.Plan, interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build Plan family: %w", err)
	}
	// read_tool_result reads back a tool output the runtime offloaded on
	// eviction. Working-directory independent (keys off the session id), so built
	// once and given to both roles. nil store → nil tool, simply omitted.
	toolResultTool, err := builtin.NewToolResultReader(config.ToolResults)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build read_tool_result: %w", err)
	}
	// Goal state is working-directory independent and keyed by session. get_goal
	// is always useful to the root Agent; report_goal_outcome is exposed only to
	// Runs stamped with a Goal incarnation at admission. That immutable provenance
	// keeps a parked deployment restorable even while the Goal is paused for HITL.
	goalGetTool, err := builtin.NewGet(config.GoalReader)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build get_goal: %w", err)
	}
	goalReportTool, err := builtin.NewReport(config.GoalReporter)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build report_goal_outcome: %w", err)
	}
	proposeSkillTool, err := builtin.NewProposal(config.SkillProposals)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build propose_skill: %w", err)
	}

	a2aTools, err = openA2AToolSet(ctx, config.A2AAgents)
	if err != nil {
		return Built{}, err
	}

	a2aExecutables := a2aTools.Tools()
	for index, executable := range a2aExecutables {
		ref, err := tool.A2A(config.A2AAgents[index].Name)
		if err != nil {
			return Built{}, err
		}
		a2aExecutables[index], err = WithIdentity(executable, ref, config.A2AAgents[index].AuthorityFingerprint())
		if err != nil {
			return Built{}, err
		}
	}

	resolver, err := newResolver(resolverDeps{
		SkillUsage:    config.SkillUsage,
		SkillsUserDir: config.SkillsUserDir, PackageSkills: config.PackageSkills,
		A2A:             a2aExecutables,
		Shell:           shellTools,
		AskUser:         askUserTool,
		EnterPlan:       planFamily.Enter,
		ExitPlan:        planFamily.Exit,
		Plan:            planFamily.Set,
		ToolResult:      toolResultTool,
		GoalGet:         goalGetTool,
		GoalReport:      goalReportTool,
		ProposeSkill:    proposeSkillTool,
		CodeIntel:       config.CodeIntelligence,
		ReadTracker:     tracker,
		MCPToolDisabled: config.MCPToolDisabled,
		RestoreMCP:      config.RestoreMCP,
	})
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build resolver: %w", err)
	}
	cleanupOnError = false
	return Built{
		Resolver: resolver,
		Shells:   shells,
		Closers: []func() error{
			shells.KillAll,
			a2aTools.Close,
		},
	}, nil
}
