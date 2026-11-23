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
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type BuildConfig struct {
	GoalAvailable    func(context.Context) (bool, error)
	Lifetime         context.Context
	UserHome         string
	SkillSources     promptsource.SkillSources
	SkillsUserDir    string
	CodeIntelligence CodeIntelligence
	PlanAvailable    func(context.Context) (bool, error)
	Plan             builtin.PlanUseCases
	PlanMode         builtin.PlanModePolicy
	Interrupt        runs.InterruptFunc
	ToolResults      builtin.ToolResultStore
	SkillUsage       builtin.SkillUsageRecorder
	SkillProposals   builtin.SkillProposalSubmitter
	GoalReader       goalReader
	GoalReporter     builtin.GoalOutcomeReporter

	MCPToolDisabled func(tool.Ref) bool
	RestoreMCP      func(context.Context, []tool.Ref, []plugin.Activation) ([]toolcontract.Tool, func() error, error)

	SandboxShell         bool
	SandboxReadOnlyPaths []string
}

type goalReader interface {
	builtin.GoalReader
}

type Built struct {
	Resolver *Resolver
	Shells   *exec.Shells
	Closers  []func() error
}

func Build(config BuildConfig) (_ Built, err error) {
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

	confiner, confErr := sandbox.NewConfiner(config.UserHome, config.SandboxReadOnlyPaths)
	shells, err := exec.NewShells(confiner, confErr, config.SandboxShell)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shells: %w", err)
	}
	shellTools, err := builtin.BuildShell(shells)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shell tools: %w", err)
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			err = errors.Join(err, shells.KillAll())
		}
	}()

	askUserTool, err := builtin.NewAskUser(config.Interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build ask_user: %w", err)
	}

	planFamily, err := builtin.BuildPlan(config.PlanMode, config.Plan, config.PlanAvailable, config.Interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build Plan family: %w", err)
	}
	toolResultTool, err := builtin.NewToolResultReader(config.ToolResults)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build read_tool_result: %w", err)
	}
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

	resolver, err := newResolver(resolverDeps{
		SkillUsage:    config.SkillUsage,
		SkillsUserDir: config.SkillsUserDir, SkillSources: config.SkillSources,
		Shell:           shellTools,
		AskUser:         askUserTool,
		GoalAvailable:   config.GoalAvailable,
		PlanAvailable:   config.PlanAvailable,
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
		},
	}, nil
}
