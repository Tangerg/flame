package bootstrap

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ChatResolver combines the execution and model-validation views of one provider boundary.
type ChatResolver interface {
	agentexec.InteractionChatResolver
	models.ChatModelValidator
}

// Config supplies one complete storage graph and explicit host policy for construction.
type Config struct {
	// Stores is the complete durable graph opened by persistence.Open.
	// Bootstrap never assembles individual missing-store feature variants.
	Stores *persistence.Bundle

	// BuildID identifies the running executable at durable executor boundaries.
	BuildID string

	// SessionOwnership extends Run/session lifecycle and destructive working-tree
	// admission across Runtime processes sharing one data directory.
	SessionOwnership ownership.AdmissionBackend
	// GoalDriveOwnership elects one autonomous Goal driver per Session across
	// those Runtime processes.
	GoalDriveOwnership goals.DriveOwnership
	// RecoveryOwnership elects one process to reconcile abandoned Runs before
	// Goals, preserving their accounting order across shared Runtime instances.
	RecoveryOwnership ownership.RecoveryBackend

	// ChatResolver resolves every frozen Run selection against the live provider
	// registry. Bootstrap never keeps a process-lifetime default client whose
	// credential material can outlive a provider update.
	ChatResolver ChatResolver

	// Pricing computes model usage cost for Runtime projections.
	Pricing accounting.Pricing

	// SkillsUserDir is the user-scope Agent Skills directory. Tool resolution
	// and workspace discovery consume it directly; it is not Agent execution
	// state.
	SkillsUserDir string

	// Maintenance overrides the default post-Run maintenance pipeline.
	Maintenance agentexec.RunMaintenance

	// Resources are one-shot process adapters whose ownership transfers to
	// the Runtime lifecycle when construction starts. It bounds each Close
	// call and releases resources
	// only after background tasks and execution/tool capabilities have stopped;
	// a returned error is retained as a diagnostic but cannot make a terminal
	// Close replayable.
	Resources []TerminalResource

	// Tool-environment inputs; the runtime reads these to assemble the tool
	// environment via toolset.Build and inject only its role resolver into the
	// Agent execution adapter.
	Online     toolset.OnlineConfig
	A2AAgents  []toolset.A2AAgentConfig
	LSPServers []codeintel.ServerSpec

	// SandboxShell opts the shell tools into per-command OS isolation (an
	// in-place jail rooted at the command's cwd: workspace-write only, network
	// denied, $HOME hidden, env scrubbed). Off by default; on a host with no
	// isolation backend it refuses assembly (fail-closed). SandboxReadOnlyPaths
	// re-opens declared toolchain roots below the hidden home for reads.
	SandboxShell         bool
	SandboxReadOnlyPaths []string

	// SandboxDir roots the ephemeral working copies for isolated runs — a session
	// marked Isolated runs its tools in a throwaway copy of its project under
	// this dir. Empty disables isolation (an isolated session's run is then
	// refused, fail-closed). The copies are scratch: never snapshotted.
	SandboxDir string

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required; the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderRegistry models.ProviderRegistry

	// ApprovalMode sets the initial runtime approval stance. It must be explicit;
	// [ComposeConfig] selects the product default [approval.ModeBalanced]. The
	// empty or an unknown mode fails assembly.
	ApprovalMode approval.Mode

	// Provider / Model name the runtime's default provider+model; the one a Run
	// runs against when it doesn't pick a model. providers.list / models.list
	// are served from the registry + catalog, not these.
	Provider string
	Model    string

	// HooksResolver resolves user-configured lifecycle hooks for a Run's cwd.
	// nil disables hooks; execution no-ops every hook seam. The composition root
	// builds the adapter-backed resolver from the storage home + trust store.
	HooksResolver HookResolver

	// RecipesGlobalDir is the global recipes directory (<FLAME_HOME>/runtime/recipes) the
	// recipes.list discovery layers under a project's .flame/recipes.
	// Empty means only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

	// CheckpointDir roots the per-session shadow-git repos backing run-boundary
	// file snapshots (<FLAME_HOME>/runtime/checkpoints); the checkpoint adapter enables
	// snapshots + file rollback only when git is present. Empty disables file
	// checkpoints. The composition root sets it.
	CheckpointDir string

	// UserHome is the process user's home directory. It anchors home-scoped
	// instruction discovery and is resolved once by the outer process root.
	UserHome string

	// DefaultWorkspacePath is the workspace selected when a request or saved
	// schedule does not name one. It is a product default supplied by the outer
	// process root, not the server process's current working directory.
	DefaultWorkspacePath string

	// ToolResultOffloadEnabled explicitly controls context eviction. When true,
	// ToolResultThreshold must be positive.
	ToolResultOffloadEnabled bool
	ToolResultThreshold      int
}

// validateAssemblyConfig reports whether the construction-time bundle has
// every capability required to assemble a Runtime. Bootstrap keeps this as a
// construction function: Config is data at the composition boundary, not a
// domain object with business behavior.
func validateAssemblyConfig(c Config) error {
	if c.Stores == nil {
		return errors.New("runtime: Stores is required")
	}
	if c.UserHome == "" {
		return errors.New("runtime: UserHome is required")
	}
	if !filepath.IsAbs(c.UserHome) {
		return errors.New("runtime: UserHome must be absolute")
	}
	if c.DefaultWorkspacePath == "" {
		return errors.New("runtime: DefaultWorkspacePath is required")
	}
	if !filepath.IsAbs(c.DefaultWorkspacePath) {
		return errors.New("runtime: DefaultWorkspacePath must be absolute")
	}
	for _, configuredPath := range []struct {
		name  string
		value string
	}{
		{name: "SkillsUserDir", value: c.SkillsUserDir},
		{name: "SandboxDir", value: c.SandboxDir},
		{name: "RecipesGlobalDir", value: c.RecipesGlobalDir},
		{name: "CheckpointDir", value: c.CheckpointDir},
	} {
		if configuredPath.value != "" && !filepath.IsAbs(configuredPath.value) {
			return fmt.Errorf("runtime: %s must be absolute when set", configuredPath.name)
		}
	}
	for index, configuredPath := range c.SandboxReadOnlyPaths {
		if configuredPath != "" && !filepath.IsAbs(configuredPath) {
			return fmt.Errorf("runtime: SandboxReadOnlyPaths[%d] must be absolute when set", index)
		}
	}
	if c.ChatResolver == nil {
		return errors.New("runtime: ChatResolver is required")
	}
	if _, err := runtimeidentity.ParseBuild(c.BuildID); err != nil {
		return fmt.Errorf("runtime: BuildID: %w", err)
	}
	if c.ProviderRegistry == nil {
		return errors.New("runtime: ProviderRegistry is required")
	}
	return nil
}
