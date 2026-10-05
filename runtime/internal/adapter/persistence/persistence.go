// Package persistence assembles Flame's durable storage adapters into one
// process-lifetime bundle. It is the storage-side capability adapter: [Open]
// returns a bundle while its consumers decide how to use each store.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/localruntime"
)

// Bundle holds every persistence backend opened for one runtime process. All
// durable stores share one SQLite database under DataDirectory. AgentMemory is
// the separate SQLite fact ledger + curated memory items.
type Bundle struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error

	DataDirectory string
	// IdempotencyNamespace identifies the exact durable replay store without
	// exposing its database path or contents.
	IdempotencyNamespace runtimeidentity.IdempotencyNamespace
	Transactor           func(context.Context, func(context.Context) error) error

	Sessions            *sqlitestore.SessionStore
	Runs                *sqlitestore.RunStore
	WorkspaceMutations  *WorkspaceMutationStore
	AgentMemory         *sqlitestore.AgentMemoryStore
	ExecutorCheckpoints *ExecutorCheckpointStore
	Interrupts          *InterruptStore
	Transcript          *sqlitestore.TranscriptStore
	Feedback            *sqlitestore.FeedbackStore
	Providers           *sqlitestore.ProviderStore
	MCPServers          *sqlitestore.MCPServerStore
	Installations       *sqlitestore.InstallationStore
	PluginReleases      *sqlitestore.ReleaseStore
	ChatHistory         *sqlitestore.MessageStore
	Trajectory          *TrajectoryReader
	ModelInvocations    *sqlitestore.ModelInvocationStore
	ToolInvocations     *sqlitestore.ToolInvocationStore
	ChildRunStarts      *sqlitestore.ChildRunStartReservationStore
	Plan                *sqlitestore.PlanStore
	PermissionModes     *sqlitestore.PermissionModeStore
	Goals               *sqlitestore.GoalStore
	ApprovalRules       *sqlitestore.ApprovalRuleStore
	UtilityRole         *sqlitestore.UtilityRoleStore
	Trust               *sqlitestore.TrustStore
	Schedules           *sqlitestore.ScheduleStore
	EmbeddingRole       *sqlitestore.EmbeddingRoleStore
	ToolResults         *sqlitestore.ToolResultStore
	Idempotency         *sqlitestore.IdempotencyStore
}

// Config is the process-owned filesystem snapshot persistence consumes. It has
// no environment or working-directory fallback: startup supplies every path.
type Config struct {
	DataDirectory string
}

const externalChangePollInterval = 100 * time.Millisecond

// StartExternalChangeObserver reports commits made through a different SQLite
// connection. PRAGMA data_version deliberately ignores commits on this
// Bundle's own connection, whose use cases already publish precise notices.
// The baseline is read before this method returns, so a caller can expose its
// Runtime without leaving an unobserved startup window. Cancel ctx and wait on
// the returned channel to join the observer. Each outage produces one diagnostic;
// retries resume notifications only after another successful database read.
func (b *Bundle) StartExternalChangeObserver(ctx context.Context, notify func()) (<-chan struct{}, error) {
	if b == nil || b.db == nil || notify == nil {
		return nil, errors.New("persistence: external change observer requires a Bundle and notification callback")
	}
	var previous int64
	if err := b.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&previous); err != nil {
		return nil, fmt.Errorf("persistence: read external change baseline: %w", err)
	}
	done := make(chan struct{})
	observer := &externalChangeObserver{db: b.db, notify: notify, version: previous}
	go func() {
		defer close(done)
		ticker := time.NewTicker(externalChangePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !observer.poll(ctx) {
					return
				}
			}
		}
	}()
	return done, nil
}

// externalChangeObserver holds one observer's poll state across ticks: the last
// data_version it saw, and whether the previous read failed so a persistent
// outage is reported once rather than on every retry.
type externalChangeObserver struct {
	db      *sql.DB
	notify  func()
	version int64
	failed  bool
}

// A failed fan-out must leave the version pending: there may be no later commit
// to wake subscribers. Invalidations are safe to repeat after partial delivery.
func (o *externalChangeObserver) poll(ctx context.Context) (running bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if !o.failed {
				slog.ErrorContext(ctx, "persistence: external change poll panicked",
					"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			}
			o.failed = true
			running = true
		}
	}()
	var current int64
	if err := o.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&current); err != nil {
		if ctx.Err() != nil {
			return false
		}
		if !o.failed {
			slog.ErrorContext(ctx, "persistence: observe external changes", "error", err)
		}
		o.failed = true
		return true
	}
	if current != o.version {
		o.notify()
		o.version = current
	}
	o.failed = false
	return true
}

// Open wires the persistence backends. The returned bundle owns the shared
// SQLite handle and must be closed when the runtime process stops.
func Open(ctx context.Context, config Config) (*Bundle, error) {
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
		db:                   db,
		DataDirectory:        config.DataDirectory,
		IdempotencyNamespace: idempotencyNamespace,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
		Sessions:            sqlitestore.NewSessionStore(db),
		Runs:                sqlitestore.NewRunStore(db),
		WorkspaceMutations:  NewWorkspaceMutationStore(sqlitestore.NewWorkspaceMutationStore(db)),
		AgentMemory:         sqlitestore.NewAgentMemoryStore(db),
		ExecutorCheckpoints: NewExecutorCheckpointStore(sqlitestore.NewExecutorCheckpointStore(db)),
		Interrupts:          NewInterruptStore(sqlitestore.NewInterruptStore(db)),
		Transcript:          sqlitestore.NewTranscriptStore(db),
		Feedback:            sqlitestore.NewFeedbackStore(db),
		Providers:           sqlitestore.NewProviderStore(db),
		MCPServers:          sqlitestore.NewMCPServerStore(db),
		Installations:       sqlitestore.NewInstallationStore(db),
		PluginReleases:      sqlitestore.NewReleaseStore(db),
		ChatHistory:         sqlitestore.NewMessageStore(db),
		Trajectory:          &TrajectoryReader{store: sqlitestore.NewTrajectoryStore(db)},
		ModelInvocations:    sqlitestore.NewModelInvocationStore(db),
		ToolInvocations:     sqlitestore.NewToolInvocationStore(db),
		ChildRunStarts:      sqlitestore.NewChildRunStartReservationStore(db),
		Plan:                sqlitestore.NewPlanStore(db),
		PermissionModes:     sqlitestore.NewPermissionModeStore(db),
		Goals:               sqlitestore.NewGoalStore(db),
		ApprovalRules:       sqlitestore.NewApprovalRuleStore(db),
		UtilityRole:         sqlitestore.NewUtilityRoleStore(db),
		Trust:               sqlitestore.NewTrustStore(db),
		Schedules:           sqlitestore.NewScheduleStore(db),
		EmbeddingRole:       sqlitestore.NewEmbeddingRoleStore(db),
		ToolResults:         sqlitestore.NewToolResultStore(db),
		Idempotency:         sqlitestore.NewIdempotencyStore(db),
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
	return b.db.PingContext(ctx)
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
