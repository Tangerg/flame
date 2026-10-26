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

	runtimeidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"

	sqlitebase "github.com/Tangerg/flame/packages/go-sdk/storage/sqlite"
)

// Bundle holds Runtime's host stores in one SQLite database under DataDirectory.
// Managed plugin processes open their private stores separately.
type Bundle struct {
	db                *sql.DB
	closeOnce         sync.Once
	closeErr          error
	validateDirectory func() error

	DataDirectory string
	// IdempotencyNamespace identifies the exact durable replay store without
	// exposing its database path or contents.
	IdempotencyNamespace runtimeidentity.IdempotencyNamespace
	Transactor           func(context.Context, func(context.Context) error) error
	ReadSnapshot         func(context.Context, func(context.Context) error) error

	Sessions              *SessionStore
	Runs                  *RunStore
	CommandAcceptances    *CommandAcceptanceStore
	WorkspaceMutations    *WorkspaceMutationStore
	ExecutorCheckpoints   *ExecutorCheckpointStore
	Interrupts            *InterruptStore
	Transcript            *TranscriptStore
	Feedback              *FeedbackStore
	Providers             *ProviderStore
	MCPServers            *MCPServerStore
	Installations         *InstallationStore
	PluginReleases        *ReleaseStore
	PluginCommands        *PluginCommandStore
	ProviderSelections    *ProviderSelectionStore
	PluginCompositions    *CompositionStore
	ChatHistory           *MessageStore
	Trajectory            *TrajectoryReader
	ModelInvocations      *ModelInvocationStore
	ToolInvocations       *ToolInvocationStore
	ChildRunStarts        *ChildRunStartReservationStore
	Plan                  *PlanStore
	ApprovalModes         *ModeStore
	Goals                 *GoalStore
	ApprovalRules         *ApprovalRuleStore
	UtilityRole           *UtilityRoleStore
	Trust                 *TrustStore
	EmbeddingRole         *EmbeddingRoleStore
	ToolResults           *ToolResultStore
	Idempotency           *IdempotencyStore
	AcceptedContinuations *AcceptedContinuationStore
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
	db, err := openDatabase(ctx, dataDirectory.DatabasePath())
	if err != nil {
		return nil, err
	}
	idempotencyNamespace, err := IdempotencyNamespace(ctx, db)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return &Bundle{
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
			return sqlitebase.RunInTx(ctx, db, fn)
		},
		ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
			return sqlitebase.RunReadSnapshot(ctx, db, read)
		},
		Sessions:              NewSessionStore(db),
		Runs:                  NewRunStore(db),
		CommandAcceptances:    NewCommandAcceptanceStore(db),
		WorkspaceMutations:    NewWorkspaceMutationStore(db),
		ExecutorCheckpoints:   NewExecutorCheckpointStore(db),
		Interrupts:            NewInterruptStore(db),
		Transcript:            NewTranscriptStore(db),
		Feedback:              NewFeedbackStore(db),
		Providers:             NewProviderStore(db),
		MCPServers:            NewMCPServerStore(db),
		Installations:         NewInstallationStore(db),
		PluginReleases:        NewReleaseStore(db),
		PluginCommands:        NewPluginCommandStore(db),
		ProviderSelections:    NewProviderSelectionStore(db),
		PluginCompositions:    NewCompositionStore(db),
		ChatHistory:           NewMessageStore(db),
		Trajectory:            &TrajectoryReader{store: newTrajectoryStore(db)},
		ModelInvocations:      NewModelInvocationStore(db),
		ToolInvocations:       NewToolInvocationStore(db),
		ChildRunStarts:        NewChildRunStartReservationStore(db),
		Plan:                  NewPlanStore(db),
		ApprovalModes:         NewModeStore(db),
		Goals:                 NewGoalStore(db),
		ApprovalRules:         NewApprovalRuleStore(db),
		UtilityRole:           NewUtilityRoleStore(db),
		Trust:                 NewTrustStore(db),
		EmbeddingRole:         NewEmbeddingRoleStore(db),
		ToolResults:           NewToolResultStore(db),
		Idempotency:           NewIdempotencyStore(db),
		AcceptedContinuations: NewAcceptedContinuationStore(db),
	}, nil
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
		if b.db != nil {
			b.closeErr = b.db.Close()
		}
	})
	return b.closeErr
}

func (b *Bundle) MCPAuthorization() *MCPAuthorizationStore {
	return NewMCPAuthorizationStore(b.db)
}
