package bootstrap

import (
	"context"
	"errors"
	"fmt"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"path/filepath"

	"github.com/Tangerg/flame/qimu/process"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/samber/lo"
)

type modelProviderAdapters interface {
	Profiles(context.Context) ([]modelprovider.Profile, error)
	RemoteModels(context.Context, modelprovider.Connection) ([]string, error)
	OpenChat(context.Context, modelchoice.Selection, modelprovider.Connection) (modeladapter.ResolvedChat, error)
	RestoreChat(context.Context, modelchoice.Selection, *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error)
	OpenEmbedding(context.Context, modelchoice.Selection, modelprovider.Connection) (modeladapter.ResolvedEmbedding, error)
}

// Config supplies one complete storage graph and explicit host policy for construction.
type assemblyConfig struct {
	// Stores is the complete durable graph opened by persistence.Open.
	// Bootstrap never assembles individual missing-store feature variants.
	stores              *persistence.Bundle
	processRunner       process.Runner
	firstPartyDirectory string

	// BuildID identifies the running executable at durable executor boundaries.
	buildID string

	// ModelProviders optionally substitutes the complete external adapter boundary.
	// Production resolves the selected installed provider after MCP composition.
	modelProviders modelProviderAdapters

	// Pricing computes model usage cost for Runtime projections.
	pricing accounting.Pricing

	// SkillsUserDir is the user-scope Agent Skills directory. Tool resolution
	// and workspace discovery consume it directly; it is not Agent execution
	// state.
	skillsUserDir string

	// Maintenance overrides the default post-Run maintenance pipeline.
	maintenance executionadapter.RunMaintenance

	// Resources are one-shot process adapters whose ownership transfers to
	// the Runtime lifecycle when construction starts. It bounds each Close
	// call and releases resources
	// only after background tasks and execution/tool capabilities have stopped;
	// a returned error is retained as a diagnostic but cannot make a terminal
	// Close replayable.
	resources []terminalResource

	// SandboxShell opts the shell tools into per-command OS isolation (an
	// in-place jail rooted at the command's cwd: workspace-write only, network
	// denied, $HOME hidden, env scrubbed). Off by default; on a host with no
	// isolation backend it refuses assembly (fail-closed). SandboxReadOnlyPaths
	// re-opens declared toolchain roots below the hidden home for reads.
	sandboxShell         bool
	sandboxReadOnlyPaths []string

	// SandboxDir roots the ephemeral working copies for isolated runs — a session
	// marked Isolated runs its tools in a throwaway copy of its project under
	// this dir. Empty disables isolation (an isolated session's run is then
	// refused, fail-closed). The copies are scratch: never snapshotted.
	sandboxDir string

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required; the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	providerRegistry models.ProviderRegistry

	// ApprovalMode is the runtime approval stance until a user chooses one,
	// which then persists. It must be explicit;
	// [ComposeConfig] selects the product default [approval.ModeBalanced]. The
	// empty or an unknown mode fails assembly.
	approvalMode approval.Mode

	// Provider / Model name the runtime's default provider+model; the one a Run
	// runs against when it doesn't pick a model. providers.list / models.list
	// are served from the registry + catalog, not these.
	provider string
	model    string

	// CheckpointDir roots the per-session shadow-git repos backing run-boundary
	// file snapshots (<FLAME_HOME>/runtime/checkpoints); the checkpoint adapter enables
	// snapshots + file rollback only when git is present. Empty disables file
	// checkpoints. The composition root sets it.
	checkpointDir string

	// UserHome is the process user's home directory. It anchors home-scoped
	// instruction discovery and is resolved once by the outer process root.
	userHome string

	// DefaultWorkspacePath is the workspace selected when a request or saved
	// schedule does not name one. It is a product default supplied by the outer
	// process root, not the server process's current working directory.
	defaultWorkspacePath string

	// ToolResultOffloadEnabled explicitly controls context eviction. When true,
	// ToolResultThreshold must be positive.
	toolResultOffloadEnabled bool
	toolResultThreshold      int
}

// validateAssemblyConfig reports whether the construction-time bundle has
// every capability required to assemble a Runtime. Bootstrap keeps this as a
// construction function: Config is data at the composition boundary, not a
// domain object with business behavior. Rejection precedes startup reconciliation
// so an unusable collaborator cannot authorize changes to durable state.
func validateAssemblyConfig(c assemblyConfig) error {
	if c.stores == nil {
		return errors.New("runtime: Stores is required")
	}
	if lo.IsNil(c.processRunner) {
		return errors.New("runtime: process runner is required")
	}
	if c.userHome == "" {
		return errors.New("runtime: UserHome is required")
	}
	if !filepath.IsAbs(c.userHome) {
		return errors.New("runtime: UserHome must be absolute")
	}
	if c.defaultWorkspacePath == "" {
		return errors.New("runtime: DefaultWorkspacePath is required")
	}
	if !filepath.IsAbs(c.defaultWorkspacePath) {
		return errors.New("runtime: DefaultWorkspacePath must be absolute")
	}
	if c.skillsUserDir == "" {
		return errors.New("runtime: SkillsUserDir is required")
	}
	if !filepath.IsAbs(c.skillsUserDir) {
		return errors.New("runtime: SkillsUserDir must be absolute")
	}
	for _, configuredPath := range []struct {
		name  string
		value string
	}{
		{name: "SandboxDir", value: c.sandboxDir},
		{name: "CheckpointDir", value: c.checkpointDir},
	} {
		if configuredPath.value != "" && !filepath.IsAbs(configuredPath.value) {
			return fmt.Errorf("runtime: %s must be absolute when set", configuredPath.name)
		}
	}
	for index, configuredPath := range c.sandboxReadOnlyPaths {
		if configuredPath != "" && !filepath.IsAbs(configuredPath) {
			return fmt.Errorf("runtime: SandboxReadOnlyPaths[%d] must be absolute when set", index)
		}
	}
	if c.modelProviders != nil && lo.IsNil(c.modelProviders) {
		return errors.New("runtime: model providers are typed nil")
	}
	if _, err := runs.ParseBuild(c.buildID); err != nil {
		return fmt.Errorf("runtime: BuildID: %w", err)
	}
	if lo.IsNil(c.providerRegistry) {
		return errors.New("runtime: ProviderRegistry is required")
	}
	return nil
}
