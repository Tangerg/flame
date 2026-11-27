package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

const commandCrashExit = 73

type commandCrashPoint string

const (
	noCommandCrash    commandCrashPoint = ""
	crashAfterClaim   commandCrashPoint = "claim"
	crashAfterReceipt commandCrashPoint = "receipt_write"
	crashAfterCommit  commandCrashPoint = "receipt_commit"
)

type atomicReceiptCommitCrashStore struct{ operation.ReplayStore }

func (s atomicReceiptCommitCrashStore) Transact(ctx context.Context, write func(context.Context) error) error {
	if err := s.ReplayStore.Transact(ctx, write); err != nil {
		return err
	}
	os.Exit(commandCrashExit)
	return nil
}

type pausedReceiptCommit struct {
	operation.ReplayStore
	committed chan<- struct{}
	release   <-chan struct{}
}

func (s pausedReceiptCommit) Transact(ctx context.Context, write func(context.Context) error) error {
	if err := s.ReplayStore.Transact(ctx, write); err != nil {
		return err
	}
	close(s.committed)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type commandCrashStore struct {
	operation.ReplayStore

	phase commandCrashPoint
}

func (s commandCrashStore) Claim(ctx context.Context, claim operation.ReplayClaim) (operation.ReplayRecord, bool, error) {
	record, claimed, err := s.ReplayStore.Claim(ctx, claim)
	if err == nil && claimed && s.phase == crashAfterClaim {
		os.Exit(commandCrashExit)
	}
	return record, claimed, err
}

func (s commandCrashStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	settled, err := s.ReplayStore.Complete(ctx, record)
	if err == nil && s.phase == crashAfterReceipt {
		os.Exit(commandCrashExit)
	}
	return settled, err
}

const embeddingRoleCrashVariable = "FLAME_TEST_EMBEDDING_ROLE_CRASH"

var originalEmbeddingRole = protocol.EmbeddingRole{Provider: "openai-compatible", Model: "original-embedding"}

func clearEmbeddingRole(ctx context.Context, endpoint *delivery.Endpoint) (*protocol.EmbeddingRole, error) {
	return endpoint.Call[protocol.EmbeddingRole, *protocol.EmbeddingRole](ctx, delivery.ModelsSetEmbeddingRole, protocol.EmbeddingRole{},
		protocol.CallMetadata{IdempotencyKey: "clear-original-embedding-role"})
}

func TestEmbeddingRoleCrashHelper(t *testing.T) {
	phase := commandCrashPoint(os.Getenv(embeddingRoleCrashVariable))
	if phase == noCommandCrash {
		return
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	role, err := modelref.NewRole(originalEmbeddingRole.Provider, originalEmbeddingRole.Model)
	if err != nil {
		t.Fatal(err)
	}
	if err := stores.EmbeddingRole.Save(t.Context(), role); err != nil {
		t.Fatal(err)
	}
	host := assembleTestRuntime(t, protocolRuntimeConfig(t, stores, newReplyStub("unused")))
	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	var replay operation.ReplayStore
	switch phase {
	case crashAfterClaim, crashAfterReceipt:
		replay = commandCrashStore{ReplayStore: stores.Idempotency, phase: phase}
	case crashAfterCommit:
		replay = atomicReceiptCommitCrashStore{ReplayStore: stores.Idempotency}
	default:
		t.Fatalf("unknown embedding role crash point %q", phase)
	}
	if err := startTestRuntimeWithReplayStore(t, host, api, stores, replay); err != nil {
		t.Fatal(err)
	}
	_, err = clearEmbeddingRole(t.Context(), host.Endpoint())
	t.Fatalf("crash point %s was not reached: %v", phase, err)
}

func TestEmbeddingRoleAndItsReceiptSurviveAbruptProcessExit(t *testing.T) {
	for _, phase := range []commandCrashPoint{crashAfterClaim, crashAfterReceipt, crashAfterCommit} {
		t.Run(string(phase), func(t *testing.T) {
			home := testsupport.TempDirectory(t)
			t.Setenv("FLAME_HOME", home)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmbeddingRoleCrashHelper$")
			command.Env = append(os.Environ(), embeddingRoleCrashVariable+"="+string(phase))
			output, err := command.CombinedOutput()
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
				t.Fatalf("abrupt exit: %v\n%s", err, output)
			}
			host, api := openProtocolRuntime(t, newReplyStub("unused"))
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			want := originalEmbeddingRole
			if phase == crashAfterCommit {
				want = protocol.EmbeddingRole{}
			}
			if current, err := api.GetEmbeddingRole(t.Context()); err != nil || current == nil || *current != want {
				t.Fatalf("embedding role after crash = %+v, %v, want %+v", current, err, want)
			}
			if phase == crashAfterCommit {
				questionCrashDatabase(t, home, `CREATE TRIGGER refuse_repeated_embedding_change
					BEFORE UPDATE ON embedding_role
					BEGIN SELECT RAISE(ABORT, 'embedding role already settled'); END`)
			}
			if result, err := clearEmbeddingRole(t.Context(), host.Endpoint()); err != nil || result == nil || *result != (protocol.EmbeddingRole{}) {
				t.Fatalf("original clear command = %+v, %v", result, err)
			}
			if current, err := api.GetEmbeddingRole(t.Context()); err != nil || current == nil || *current != (protocol.EmbeddingRole{}) {
				t.Fatalf("embedding role after replay = %+v, %v, want unconfigured", current, err)
			}
		})
	}
}

func TestUtilityRolePublishesOnlyCommittedChanges(t *testing.T) {
	const setRole delivery.Name = "models.setUtilityRole"
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	host, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	original := protocol.UtilityRole{Provider: "anthropic", Model: "claude-opus-5"}
	if _, err := api.SetUtilityRole(ctx, original); err != nil {
		t.Fatal(err)
	}
	clear := func() (*protocol.UtilityRole, error) {
		return host.Endpoint().Call[protocol.UtilityRole, *protocol.UtilityRole](ctx, setRole, protocol.UtilityRole{},
			protocol.CallMetadata{IdempotencyKey: "clear-original-utility-role"})
	}
	expectRole := func(want protocol.UtilityRole) {
		t.Helper()
		if current, err := api.GetUtilityRole(ctx); err != nil || current == nil || *current != want {
			t.Fatalf("live utility role = %+v, %v, want %+v", current, err, want)
		}
	}
	allowReceipt := refuseCommandReceipt(t, home)
	result, err := clear()
	allowReceipt()
	if result != nil || err == nil {
		t.Fatalf("clear utility role with a refused receipt = %+v, %v", result, err)
	}
	expectRole(original)
	if result, err := clear(); err != nil || result == nil || *result != (protocol.UtilityRole{}) {
		t.Fatalf("original clear command = %+v, %v", result, err)
	}
	expectRole(protocol.UtilityRole{})
	if _, err := api.SetUtilityRole(ctx, original); err != nil {
		t.Fatal(err)
	}
	if result, err := clear(); err != nil || result == nil || *result != (protocol.UtilityRole{}) {
		t.Fatalf("replayed clear command = %+v, %v", result, err)
	}
	expectRole(original)
}

func TestEmbeddingRoleReadObservesCommittedSelection(t *testing.T) {
	home := testsupport.TempDirectory(t)
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	original, err := modelref.NewRole(originalEmbeddingRole.Provider, originalEmbeddingRole.Model)
	if err != nil {
		t.Fatal(err)
	}
	if err := stores.EmbeddingRole.Save(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	host := assembleTestRuntime(t, protocolRuntimeConfig(t, stores, newReplyStub("unused")))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	committed, release := make(chan struct{}), make(chan struct{})
	if err := startTestRuntimeWithReplayStore(t, host, api, stores, pausedReceiptCommit{stores.Idempotency, committed, release}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer cancel()
	written := make(chan error, 1)
	go func() {
		_, err := clearEmbeddingRole(ctx, host.Endpoint())
		written <- err
	}()
	select {
	case <-committed:
	case err := <-written:
		t.Fatalf("clear role ended without reaching its commit: %v", err)
	case <-ctx.Done():
		t.Fatal("clear role did not commit")
	}
	current, err := host.Endpoint().Call[struct{}, *protocol.EmbeddingRole](ctx, "models.getEmbeddingRole", struct{}{}, protocol.CallMetadata{})
	close(release)
	if writeErr := <-written; writeErr != nil {
		t.Fatalf("clear embedding role: %v", writeErr)
	}
	if err != nil || current == nil || *current != (protocol.EmbeddingRole{}) {
		t.Fatalf("embedding role after commit = %+v, %v, want unconfigured", current, err)
	}
}

func refuseCommandReceipt(t *testing.T, home string) func() {
	t.Helper()
	questionCrashDatabase(t, home, `CREATE TRIGGER refuse_command_receipt
		BEFORE UPDATE OF payload ON idempotency_records
		BEGIN SELECT RAISE(ABORT, 'command receipt refused'); END`)
	return func() {
		t.Helper()
		questionCrashDatabase(t, home, "DROP TRIGGER refuse_command_receipt")
	}
}
