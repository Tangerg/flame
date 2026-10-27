// Package persistence assembles Flame's durable storage adapters into one
// process-lifetime bundle. It is the storage-side capability adapter: [Open]
// returns a bundle while its consumers decide how to use each store.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

// Bundle holds Runtime's host stores in one SQLite database under DataDirectory.
// Managed plugin processes open their private stores separately.
type Bundle struct {
	db                *sql.DB
	Qimu              *sqlite.Store
	Admission         *qimu.Admission
	PluginUses        *sqlite.Owner
	closeOnce         sync.Once
	closeErr          error
	validateDirectory func() error

	DataDirectory string
	// IdempotencyNamespace identifies the exact durable replay store without
	// exposing its database path or contents.
	IdempotencyNamespace   operation.ReplayNamespace
	Transactor             func(context.Context, func(context.Context) error) error
	ReadSnapshot           func(context.Context, func(context.Context) error) error
	CommitPluginReferences PluginReferenceTransactor

	Sessions              *SessionStore
	Runs                  *RunStore
	CommandAcceptances    *commandAcceptanceStore
	WorkspaceMutations    *WorkspaceMutationStore
	ExecutorCheckpoints   *ExecutorCheckpointStore
	Interrupts            *InterruptStore
	Transcript            *TranscriptStore
	Feedback              *feedbackStore
	Providers             *ProviderStore
	MCPServers            *MCPServerStore
	Installations         *InstallationStore
	PluginReleases        *ReleaseStore
	PluginCommands        *pluginCommandStore
	ProviderSelections    *providerSelectionStore
	PluginCompositions    *compositionStore
	ChatHistory           *MessageStore
	Trajectory            *trajectoryReader
	ModelInvocations      *ModelInvocationStore
	ToolInvocations       *ToolInvocationStore
	ChildRunStarts        *ChildRunStartReservationStore
	Plan                  *PlanStore
	ApprovalModes         *ModeStore
	Goals                 *GoalStore
	ApprovalRules         *ApprovalRuleStore
	UtilityRole           *utilityRoleStore
	Trust                 *trustStore
	EmbeddingRole         *embeddingRoleStore
	ToolResults           *ToolResultStore
	Idempotency           *idempotencyStore
	AcceptedContinuations *acceptedContinuationStore
}

// Config is the process-owned filesystem snapshot persistence consumes. It has
// no environment or working-directory fallback: startup supplies every path.
type Config struct {
	DataDirectory     string
	ValidateDirectory func() error
}

// Open wires the persistence backends. The returned bundle owns the shared
// SQLite handle and must be closed when the runtime process stops.
func Open(ctx context.Context, config Config) (*Bundle, error) {
	if config.ValidateDirectory != nil {
		if err := config.ValidateDirectory(); err != nil {
			return nil, err
		}
	}
	dataDirectory, directoryErr := localruntime.DataDirectoryAt(config.DataDirectory)
	if directoryErr != nil {
		return nil, fmt.Errorf("persistence: data directory: %w", directoryErr)
	}
	config.DataDirectory = dataDirectory.Path()
	if mkdirErr := os.MkdirAll(config.DataDirectory, 0o700); mkdirErr != nil {
		return nil, fmt.Errorf("persistence: create data directory %q: %w", config.DataDirectory, mkdirErr)
	}
	if chmodErr := os.Chmod(config.DataDirectory, 0o700); chmodErr != nil {
		return nil, fmt.Errorf("persistence: protect data directory %q: %w", config.DataDirectory, chmodErr)
	}
	store, admission, err := openFrameworkDatabase(ctx, dataDirectory.DatabasePath())
	if err != nil {
		return nil, err
	}
	db := store.Database()
	idempotencyNamespace, err := idempotencyNamespace(ctx, db)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	uses, err := store.Owner("flame")
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	bundle := &Bundle{
		Qimu: store, Admission: admission, PluginUses: uses,
		validateDirectory:    config.ValidateDirectory,
		db:                   db,
		DataDirectory:        config.DataDirectory,
		IdempotencyNamespace: idempotencyNamespace,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			if config.ValidateDirectory != nil {
				if err := config.ValidateDirectory(); err != nil {
					return err
				}
			}
			return sqlite.RunInTx(ctx, db, fn)
		},
		ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
			return sqlite.RunReadSnapshot(ctx, db, read)
		},
		Sessions:              newSessionStore(db),
		Runs:                  newRunStore(db),
		CommandAcceptances:    newCommandAcceptanceStore(db),
		WorkspaceMutations:    newWorkspaceMutationStore(db),
		ExecutorCheckpoints:   newExecutorCheckpointStore(db),
		Interrupts:            newInterruptStore(db),
		Transcript:            newTranscriptStore(db),
		Feedback:              newFeedbackStore(db),
		Providers:             newProviderStore(db),
		MCPServers:            newMCPServerStore(db),
		Installations:         newInstallationStore(db),
		PluginReleases:        newReleaseStore(db),
		PluginCommands:        newPluginCommandStore(db, nil),
		ProviderSelections:    newProviderSelectionStore(db),
		PluginCompositions:    newCompositionStore(db),
		ChatHistory:           newMessageStore(db),
		Trajectory:            &trajectoryReader{store: newTrajectoryStore(db)},
		ModelInvocations:      newModelInvocationStore(db),
		ToolInvocations:       newToolInvocationStore(db),
		ChildRunStarts:        newChildRunStartReservationStore(db),
		Plan:                  newPlanStore(db),
		ApprovalModes:         newModeStore(db),
		Goals:                 newGoalStore(db),
		ApprovalRules:         newApprovalRuleStore(db),
		UtilityRole:           newUtilityRoleStore(db),
		Trust:                 newTrustStore(db),
		EmbeddingRole:         newEmbeddingRoleStore(db),
		ToolResults:           newToolResultStore(db),
		Idempotency:           newIdempotencyStore(db),
		AcceptedContinuations: newAcceptedContinuationStore(db),
	}
	bundle.PluginCommands.commit = bundle.commitPluginWrite
	bundle.CommitPluginReferences = bundle.commitPluginReferences
	bundle.ExecutorCheckpoints.admission = admission
	return bundle, nil
}

// Ping reports whether the shared SQLite handle can still serve a statement.
// The pool serializes on one connection, so a bundle busy with live work
// returns ctx's error rather than a storage failure; callers that report health
// must keep the two apart.
func (b *Bundle) Ping(ctx context.Context) error {
	if b == nil || b.db == nil {
		return errors.New("persistence: storage is closed")
	}
	if err := b.ValidateDirectory(); err != nil {
		return err
	}
	return b.db.PingContext(ctx)
}

func (b *Bundle) ValidateDirectory() error {
	if b.validateDirectory != nil {
		return b.validateDirectory()
	}
	return nil
}

// Close releases the shared SQLite handle. It is safe to call repeatedly.
func (b *Bundle) Close() error {
	if b == nil {
		return nil
	}
	b.closeOnce.Do(func() {
		if b.Qimu != nil {
			b.closeErr = b.Qimu.Close()
		}
	})
	return b.closeErr
}

func (b *Bundle) MCPAuthorization() *mcpAuthorizationStore {
	return newMCPAuthorizationStore(b.db)
}
