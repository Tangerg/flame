package bootstrap

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/feedback"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type readServices struct {
	workspaceFiles     *workspace.Files
	workspaceVCS       *workspace.VCS
	workspaceDiscovery *workspace.Discovery
	workspaceWatch     *workspace.Watch
	modelEndpoints     *models.EndpointObservations
	queries            *sessions.QueryCoordinator
	usage              *sessions.UsageReporter
	feedback           *feedback.Recorder
	trajectoryExports  *sessions.TrajectoryExporter
}

func assembleReadServices(
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	workspaceServices workspaceComposition,
	execution executionComposition,
	sessionCoordinator *sessions.Coordinator,
	sessionStores *persistence.SessionStores,
) (readServices, error) {
	workspaceFiles, err := workspace.NewFiles(workspaceServices.scope, workspaceadapter.FileBrowser{})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace files: %w", err)
	}
	workspaceVCS, err := workspace.NewVCS(workspaceServices.scope, workspaceadapter.VCS{})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace vcs: %w", err)
	}
	workspaceDiscovery, err := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{},
	)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace discovery: %w", err)
	}
	workspaceWatch, err := workspace.NewWatch(
		workspaceServices.scope,
		workspaceadapter.NewFileWatcher(lifetime.context),
	)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace git watch: %w", err)
	}
	modelEndpoints, err := models.NewEndpointObservations(cfg.stores.ModelInvocations)
	if err != nil {
		return readServices{}, err
	}
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		ReadSnapshot:     cfg.stores.ReadSnapshot,
		Trajectory:       cfg.stores.Trajectory,
		ModelInvocations: cfg.stores.ModelInvocations,
		Transcript:       cfg.stores.Transcript,
		Interrupts:       cfg.stores.Interrupts,
		Runs:             cfg.stores.Runs,
		Sessions:         cfg.stores.Sessions,
		Plan:             cfg.stores.Plan,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct session queries: %w", err)
	}
	usage, err := sessions.NewUsageReporter(sessions.UsageDependencies{
		Source: cfg.stores.ModelInvocations, Now: time.Now,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct usage reporter: %w", err)
	}
	recorder, err := feedback.NewRecorder(execution.tools.pluginCalls, cfg.stores.Annotations, time.Now)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct feedback recorder: %w", err)
	}
	trajectoryReader, err := persistence.NewTrajectoryExportReader(persistence.TrajectoryExportConfig{
		Snapshots: sessionStores, Sessions: cfg.stores.Sessions, ModelInvocations: cfg.stores.ModelInvocations,
		ToolInvocations: cfg.stores.ToolInvocations,
		Annotations:     cfg.stores.Annotations, ReadSnapshot: cfg.stores.ReadSnapshot,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct trajectory export reader: %w", err)
	}
	trajectoryExports, err := sessions.NewTrajectoryExporter(sessionCoordinator, trajectoryReader)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct trajectory exporter: %w", err)
	}
	return readServices{
		workspaceFiles: workspaceFiles, workspaceVCS: workspaceVCS, workspaceDiscovery: workspaceDiscovery, workspaceWatch: workspaceWatch,
		modelEndpoints: modelEndpoints, queries: queries, usage: usage, feedback: recorder, trajectoryExports: trajectoryExports,
	}, nil
}
