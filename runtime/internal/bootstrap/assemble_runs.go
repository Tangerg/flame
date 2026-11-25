package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type runFoundation struct {
	sessions    *sessions.Coordinator
	admissions  *ownership.Gate
	fileChanges func(workspace.FileChangeNotice)
}

func assembleRuns(
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
	foundation runFoundation,
) (composedRuns, error) {
	var err error
	runEffectTasks := &taskgroup.Group{}
	lifetime.runEffectTasks = runEffectTasks
	runSegmentConfig := segment.Config{
		Interrupts:          cfg.stores.Interrupts,
		Sessions:            cfg.stores.Sessions,
		Transcript:          cfg.stores.Transcript,
		ItemReplacer:        cfg.stores.Transcript,
		ModelInvocations:    cfg.stores.ModelInvocations,
		ToolInvocations:     cfg.stores.ToolInvocations,
		Conversation:        execution.conversation.store,
		Compactions:         execution.conversation.compactions,
		ContextState:        execution.transientSessions,
		State:               cfg.stores.Runs,
		RunProgress:         cfg.stores.Runs,
		ExecutorCheckpoints: cfg.stores.ExecutorCheckpoints,
		ExecutionTrees:      cfg.stores.ExecutorCheckpoints,
		ChildRunStarts:      cfg.stores.ChildRunStarts,
		Tx:                  segment.Transactor(cfg.stores.Transactor),
		PluginBindings: func(ctx context.Context, identity string, checkpoint runs.ExecutorCheckpoint, apply func(context.Context) error) error {
			return cfg.stores.CommitPluginReferences(ctx, policy.framework.host, identity, checkpoint, apply)
		},
	}
	runSegmentConfig.CommandAcceptances = cfg.stores.CommandAcceptances
	runSegmentConfig.ActorAuthority, err = plugins.NewRunActorAdmission(cfg.stores.Installations)
	if err != nil {
		return composedRuns{}, err
	}
	runSegmentConfig.GoalRuns = cfg.stores.Goals
	runSegmentConfig.RememberedRules = cfg.stores.ApprovalRules
	runSegmentConfig.AcceptedContinuations = cfg.stores.AcceptedContinuations
	runSegmentEffects, err := segment.New(runSegmentConfig)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run-segment effects: %w", err)
	}
	runFinalizer, err := segment.NewFinalizer(segment.FinalizerConfig{
		Checkpoints:   workspaceServices.checkpoints,
		Notifications: execution.workingContexts,
		Tasks:         runEffectTasks,
		Titles: segment.TitleMaintenance{
			Sessions: foundation.sessions,
			Model:    execution.models.utilityModel,
		},
	})
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run finalizer: %w", err)
	}
	workspaceNotifier, err := segment.NewWorkspaceNotifier(foundation.fileChanges)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct workspace notifier: %w", err)
	}
	runSegmentDependencies := runs.SegmentDependencies{
		Observations: execution.executor,
		Releases:     execution.executor,
		Publication: runs.PublicationPorts{
			Openings: runSegmentEffects, Events: runSegmentEffects, Barriers: runSegmentEffects, Workspace: workspaceNotifier,
		},
		ChildStarts:   runSegmentEffects,
		Finalizer:     runFinalizer,
		Runs:          cfg.stores.Runs,
		ToolAttempts:  cfg.stores.ToolInvocations,
		Admissions:    foundation.admissions,
		Now:           time.Now,
		Retention:     runs.DefaultRetention(),
		NewRunID:      newRunID,
		NewSegmentID:  newSegmentID,
		Invalidations: policy.invalidations.publish,
	}
	runDependencies := runs.Dependencies{
		CommandAcceptances:    cfg.stores.CommandAcceptances,
		Continuation:          execution.executor,
		ApprovalRules:         policy.approvals,
		AcceptedContinuations: cfg.stores.AcceptedContinuations,
		Steering:              execution.executor,
		NewItemID:             newItemID,
		Sessions:              foundation.sessions,
		Interrupts:            cfg.stores.Interrupts,
		Checkpoints:           runSegmentEffects,
		Runs:                  cfg.stores.Runs,
		Items:                 cfg.stores.Transcript,
		Admissions:            foundation.admissions,
	}
	openingDependencies := runs.OpeningDependencies{
		CommandAcceptances: cfg.stores.CommandAcceptances,
		RootStarts:         execution.executor,
		Releases:           execution.executor,
		Conversation:       execution.conversation.messages,
		WorkingContexts:    execution.workingContexts,
		Models:             execution.models.capabilities,
		ModelSelector:      execution.tools.callComposition,
		Sessions:           foundation.sessions,
		SessionCreator:     foundation.sessions,
		ActiveRuns:         foundation.sessions,
		Admissions:         foundation.admissions,
	}
	cancellationDependencies := runs.CancellationDependencies{
		RootCancellation:                   execution.executor,
		RunningSubtreeCanceler:             execution.executor,
		WaitingSubtreeCancellationPreparer: execution.executor,
		WaitingSubtreeCancellations:        runSegmentEffects,
		AcceptedSubtreeCancellations:       cfg.stores.AcceptedContinuations,
		SubtreeCancellationRestorer:        execution.executor,
		Terminations:                       foundation.sessions,
		Sessions:                           foundation.sessions,
		Interrupts:                         cfg.stores.Interrupts,
		Checkpoints:                        runSegmentEffects,
		Releases:                           execution.executor,
		Runs:                               cfg.stores.Runs,
		Items:                              cfg.stores.Transcript,
		Admissions:                         foundation.admissions,
	}
	openingDependencies.Isolation = execution.isolation
	return composeRuns(runDependencies, openingDependencies, cancellationDependencies, runSegmentDependencies)
}

type composedRuns struct {
	coordinator   *runs.Coordinator
	openings      *runs.Openings
	cancellations *runs.Cancellations
	segments      *runs.Segments
}

func composeRuns(
	deps runs.Dependencies,
	openingDeps runs.OpeningDependencies,
	cancellationDeps runs.CancellationDependencies,
	segmentDeps runs.SegmentDependencies,
) (composedRuns, error) {
	segments, err := runs.NewSegments(segmentDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run segments: %w", err)
	}
	deps.Segments, openingDeps.Segments, cancellationDeps.Segments = segments, segments, segments
	coordinator, err := runs.NewCoordinator(deps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run coordinator: %w", err)
	}
	openings, err := runs.NewOpenings(openingDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run openings: %w", err)
	}
	cancellations, err := runs.NewCancellations(cancellationDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run cancellations: %w", err)
	}
	return composedRuns{coordinator: coordinator, openings: openings, cancellations: cancellations, segments: segments}, nil
}

func startupRunRecovery(stores *persistence.Bundle, pluginCoordinator *plugins.Coordinator, runCoordinator *runs.Coordinator, recoveryPersistence *persistence.RecoveryStore, executor *executionadapter.InteractionExecutor, ownershipRecovery *ownership.RecoveryCoordinator) func(context.Context) error {
	var recoveryMu sync.Mutex
	recovered := false
	prepared := false
	var uncertain runs.UncertainRevival
	return func(ctx context.Context) error {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if recovered {
			return nil
		}
		if err := stores.Admission.RecoveryError(); err != nil {
			return err
		}
		if !prepared {
			if err := pluginCoordinator.Reclaim(ctx); err != nil {
				return fmt.Errorf("runtime: reclaim plugin content: %w", err)
			}
			if err := runCoordinator.ReviveAcceptedContinuations(ctx); err != nil {
				return fmt.Errorf("runtime: revive accepted continuations: %w", err)
			}
			if err := stores.ModelInvocations.ReconcileEndpointInvocations(ctx, time.Now().UTC()); err != nil {
				return err
			}
			prepared = true
		}
		err := uncertain.Reconcile(ctx, runCoordinator, recoveryPersistence, executor)
		if err != nil && !errors.Is(err, runs.ErrOriginalSourcePending) {
			return fmt.Errorf("runtime: revive uncertain executions: %w", err)
		}
		if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
			return fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
		}
		if err != nil {
			return err
		}
		recovered = true
		return nil
	}
}
