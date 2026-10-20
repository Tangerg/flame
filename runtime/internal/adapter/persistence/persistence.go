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
	runtimeidentity "github.com/Tangerg/flame/runtime/identity"
	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
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

	Sessions              *sqlitestore.SessionStore
	Runs                  *sqlitestore.RunStore
	CommandAcceptances    *CommandAcceptanceStore
	WorkspaceMutations    *WorkspaceMutationStore
	ExecutorCheckpoints   *ExecutorCheckpointStore
	Interrupts            *InterruptStore
	Transcript            *sqlitestore.TranscriptStore
	Feedback              *sqlitestore.FeedbackStore
	Providers             *sqlitestore.ProviderStore
	MCPServers            *sqlitestore.MCPServerStore
	Installations         *sqlitestore.InstallationStore
	PluginReleases        *sqlitestore.ReleaseStore
	PluginCommands        *sqlitestore.PluginCommandStore
	ProviderSelections    *sqlitestore.ProviderSelectionStore
	PluginCompositions    *sqlitestore.CompositionStore
	ChatHistory           *sqlitestore.MessageStore
	Trajectory            *TrajectoryReader
	ModelInvocations      *sqlitestore.ModelInvocationStore
	ToolInvocations       *sqlitestore.ToolInvocationStore
	ChildRunStarts        *sqlitestore.ChildRunStartReservationStore
	Plan                  *sqlitestore.PlanStore
	ApprovalModes         *sqlitestore.ModeStore
	Goals                 *sqlitestore.GoalStore
	ApprovalRules         *sqlitestore.ApprovalRuleStore
	UtilityRole           *sqlitestore.UtilityRoleStore
	Trust                 *sqlitestore.TrustStore
	EmbeddingRole         *sqlitestore.EmbeddingRoleStore
	ToolResults           *sqlitestore.ToolResultStore
	Idempotency           *sqlitestore.IdempotencyStore
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
	db, err := sqlitestore.Open(ctx, dataDirectory.DatabasePath())
	if err != nil {
		return nil, err
	}
	idempotencyNamespace, err := sqlitestore.IdempotencyNamespace(ctx, db)
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
		Sessions:              sqlitestore.NewSessionStore(db),
		Runs:                  sqlitestore.NewRunStore(db),
		CommandAcceptances:    NewCommandAcceptanceStore(db),
		WorkspaceMutations:    NewWorkspaceMutationStore(sqlitestore.NewWorkspaceMutationStore(db)),
		ExecutorCheckpoints:   NewExecutorCheckpointStore(sqlitestore.NewExecutorCheckpointStore(db)),
		Interrupts:            NewInterruptStore(sqlitestore.NewInterruptStore(db)),
		Transcript:            sqlitestore.NewTranscriptStore(db),
		Feedback:              sqlitestore.NewFeedbackStore(db),
		Providers:             sqlitestore.NewProviderStore(db),
		MCPServers:            sqlitestore.NewMCPServerStore(db),
		Installations:         sqlitestore.NewInstallationStore(db),
		PluginReleases:        sqlitestore.NewReleaseStore(db),
		PluginCommands:        sqlitestore.NewPluginCommandStore(db),
		ProviderSelections:    sqlitestore.NewProviderSelectionStore(db),
		PluginCompositions:    sqlitestore.NewCompositionStore(db),
		ChatHistory:           sqlitestore.NewMessageStore(db),
		Trajectory:            &TrajectoryReader{store: sqlitestore.NewTrajectoryStore(db)},
		ModelInvocations:      sqlitestore.NewModelInvocationStore(db),
		ToolInvocations:       sqlitestore.NewToolInvocationStore(db),
		ChildRunStarts:        sqlitestore.NewChildRunStartReservationStore(db),
		Plan:                  sqlitestore.NewPlanStore(db),
		ApprovalModes:         sqlitestore.NewModeStore(db),
		Goals:                 sqlitestore.NewGoalStore(db),
		ApprovalRules:         sqlitestore.NewApprovalRuleStore(db),
		UtilityRole:           sqlitestore.NewUtilityRoleStore(db),
		Trust:                 sqlitestore.NewTrustStore(db),
		EmbeddingRole:         sqlitestore.NewEmbeddingRoleStore(db),
		ToolResults:           sqlitestore.NewToolResultStore(db),
		Idempotency:           sqlitestore.NewIdempotencyStore(db),
		AcceptedContinuations: NewAcceptedContinuationStore(sqlitestore.NewAcceptedContinuationStore(db)),
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

func (b *Bundle) MCPAuthorization() *sqlitestore.MCPAuthorizationStore {
	return sqlitestore.NewMCPAuthorizationStore(b.db)
}
