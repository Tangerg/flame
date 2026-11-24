package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/application/operation"

	framehost "github.com/Tangerg/frame/host"
	"github.com/Tangerg/frame/sqlite"
)

type Bundle struct {
	db                *database
	Frame             sqlite.Store
	Admission         framehost.Admission
	PluginUses        sqlite.Owner
	closeOnce         sync.Once
	closeErr          error
	validateDirectory func() error

	DataDirectory          string
	IdempotencyNamespace   operation.ReplayNamespace
	Transactor             Transactor
	ReadSnapshot           Transactor
	CommitPluginReferences PluginReferenceTransactor

	Sessions              *SessionStore
	Projects              *ProjectStore
	Runs                  *RunStore
	CommandAcceptances    *commandAcceptanceStore
	WorkspaceMutations    *WorkspaceMutationStore
	ResourceRetirements   *ResourceRetirementStore
	ExecutorCheckpoints   *ExecutorCheckpointStore
	Interrupts            *InterruptStore
	Transcript            *TranscriptStore
	Annotations           *annotationStore
	Providers             *ProviderStore
	MCPServers            *MCPServerStore
	Installations         *InstallationStore
	PluginReleases        *ReleaseStore
	PluginCommands        *pluginCommandStore
	ProviderSelections    *providerSelectionStore
	PluginCompositions    *compositionStore
	Conversations         *ConversationStore
	Trajectory            *trajectoryStore
	ModelInvocations      *ModelInvocationStore
	ToolInvocations       *ToolInvocationStore
	ChildRunStarts        *ChildRunStartReservationStore
	Plan                  *PlanStore
	ApprovalModes         *ModeStore
	Goals                 *GoalStore
	ApprovalRules         *ApprovalRuleStore
	UtilityRole           *RoleStore
	Trust                 *trustStore
	EmbeddingRole         *RoleStore
	ToolResults           *ToolResultStore
	Idempotency           *idempotencyStore
	AcceptedContinuations *acceptedContinuationStore
}

type Transactor func(context.Context, func(context.Context) error) error

type Config struct {
	DataDirectory     string
	ValidateDirectory func() error
}

func Open(ctx context.Context, config Config) (*Bundle, error) {
	if config.ValidateDirectory == nil {
		return nil, errors.New("persistence: data directory ownership check is required")
	}
	if err := config.ValidateDirectory(); err != nil {
		return nil, err
	}
	dataDirectory, directoryErr := localruntime.DataDirectoryAt(config.DataDirectory)
	if directoryErr != nil {
		return nil, fmt.Errorf("persistence: data directory: %w", directoryErr)
	}
	config.DataDirectory = dataDirectory.Path()
	if mkdirErr := os.MkdirAll(config.DataDirectory, filesystem.PrivateDirectoryMode); mkdirErr != nil {
		return nil, fmt.Errorf("persistence: create data directory %q: %w", config.DataDirectory, mkdirErr)
	}
	if chmodErr := os.Chmod(config.DataDirectory, filesystem.PrivateDirectoryMode); chmodErr != nil {
		return nil, fmt.Errorf("persistence: protect data directory %q: %w", config.DataDirectory, chmodErr)
	}
	store, admission, installations, err := openFrameworkDatabase(ctx, dataDirectory.DatabasePath())
	if err != nil {
		return nil, err
	}
	db := installations.db
	idempotencyNamespace, err := idempotencyNamespace(ctx, db)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	uses, err := store.Owner(flameOwner)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	receipts := newIdempotencyStore(db)
	bundle := &Bundle{
		Frame: store, Admission: admission, PluginUses: uses,
		validateDirectory:    config.ValidateDirectory,
		db:                   db,
		DataDirectory:        config.DataDirectory,
		IdempotencyNamespace: idempotencyNamespace,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			if err := config.ValidateDirectory(); err != nil {
				return err
			}
			return db.write(ctx, fn)
		},
		ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
			return db.read(ctx, read)
		},
		Sessions:              newSessionStore(db),
		Projects:              &ProjectStore{db: db},
		Runs:                  newRunStore(db),
		CommandAcceptances:    newCommandAcceptanceStore(db),
		WorkspaceMutations:    &WorkspaceMutationStore{db: db, receipts: receipts},
		ResourceRetirements:   newResourceRetirementStore(db),
		ExecutorCheckpoints:   newExecutorCheckpointStore(db, admission),
		Interrupts:            newInterruptStore(db),
		Transcript:            newTranscriptStore(db),
		Annotations:           &annotationStore{db: db},
		Providers:             newProviderStore(db),
		MCPServers:            newMCPServerStore(db),
		Installations:         installations,
		PluginReleases:        installations.releases,
		ProviderSelections:    newProviderSelectionStore(db),
		PluginCompositions:    newCompositionStore(db),
		Conversations:         newConversationStore(db),
		Trajectory:            newTrajectoryStore(db),
		ModelInvocations:      newModelInvocationStore(db),
		ToolInvocations:       newToolInvocationStore(db),
		ChildRunStarts:        newChildRunStartReservationStore(db),
		Plan:                  newPlanStore(db),
		ApprovalModes:         newModeStore(db),
		Goals:                 newGoalStore(db),
		ApprovalRules:         newApprovalRuleStore(db),
		UtilityRole:           newRoleStore(db, "utility_role", "utility role"),
		Trust:                 newTrustStore(db),
		EmbeddingRole:         newRoleStore(db, "embedding_role", "embedding role"),
		ToolResults:           newToolResultStore(db),
		Idempotency:           receipts,
		AcceptedContinuations: newAcceptedContinuationStore(db),
	}
	bundle.PluginCommands = newPluginCommandStore(db, bundle.commitPluginWrite)
	bundle.CommitPluginReferences = bundle.commitPluginReferences
	return bundle, nil
}

// The pool serializes on one connection, so a busy bundle returns ctx's error
// rather than a storage failure.
func (b *Bundle) Ping(ctx context.Context) error {
	if b == nil || b.db == nil {
		return errors.New("persistence: storage is closed")
	}
	if err := b.ValidateDirectory(); err != nil {
		return err
	}
	return b.db.read(ctx, func(ctx context.Context) error {
		var one int
		return b.db.connection(ctx).QueryRowContext(ctx, "SELECT 1").Scan(&one)
	})
}

func (b *Bundle) ValidateDirectory() error { return b.validateDirectory() }

func (b *Bundle) Close() error {
	if b == nil {
		return nil
	}
	b.closeOnce.Do(func() {
		if b.Frame.Valid() {
			b.closeErr = b.Frame.Close()
		}
	})
	return b.closeErr
}

func (b *Bundle) MCPAuthorization() *mcpAuthorizationStore {
	return newMCPAuthorizationStore(b.db)
}
