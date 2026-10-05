package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/protocol"
)

const commandCrashExit = 73

type commandCrashStore struct {
	idempotency.Store
	phase string
}

func (s commandCrashStore) Claim(ctx context.Context, key, fingerprint string) (idempotency.Record, bool, error) {
	record, claimed, err := s.Store.Claim(ctx, key, fingerprint)
	if err == nil && claimed && s.phase == "claim" {
		os.Exit(commandCrashExit)
	}
	return record, claimed, err
}

func (s commandCrashStore) Complete(ctx context.Context, record idempotency.Record) error {
	err := s.Store.Complete(ctx, record)
	if err == nil && s.phase == "receipt" {
		os.Exit(commandCrashExit)
	}
	return err
}

// commandCrashExecutor records business effects in a database of its own, so
// the Runtime's directory keeps exactly its current schema.
type commandCrashExecutor struct {
	effects *sql.DB
	phase   string
}

func (s commandCrashExecutor) SteerRun(ctx context.Context, _ protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	// The unconstrained fixture records every execution, so replay cannot hide a
	// duplicate behind a business uniqueness check. It is not a Run simulator.
	if _, err := s.effects.ExecContext(ctx, `INSERT INTO command_crash_effects (item_id) VALUES ('item_crash')`); err != nil {
		return nil, err
	}
	if s.phase == "business" {
		os.Exit(commandCrashExit)
	}
	return &protocol.SteerRunResponse{UserItemID: "item_crash"}, nil
}

func crashCommand(t *testing.T, db, effects *sql.DB, phase string) (*protocol.SteerRunResponse, error) {
	t.Helper()
	namespace, err := sqlite.IdempotencyNamespace(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := delivery.NewEndpoint(commandCrashExecutor{effects: effects, phase: phase}, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: namespace.String(),
		IdempotencyStore: commandCrashStore{Store: sqlite.NewIdempotencyStore(db), phase: phase},
	})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint.Call[protocol.SteerRunRequest, *protocol.SteerRunResponse](t.Context(), delivery.RunsSteer,
		protocol.SteerRunRequest{
			RunID: "run_crash", ExpectedSegmentID: "seg_crash",
			Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "once"}},
		}, delivery.Options{IdempotencyKey: "crash-command", IdempotencyNamespace: namespace.String()})
}

func TestCommandCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_COMMAND_CRASH")
	if phase == "" {
		return
	}
	db, err := sqlite.Open(t.Context(), os.Getenv("FLAME_TEST_COMMAND_DB"))
	if err != nil {
		t.Fatal(err)
	}
	effects := openCommandEffects(t, os.Getenv("FLAME_TEST_COMMAND_EFFECTS"))
	if _, err := effects.ExecContext(t.Context(), `CREATE TABLE command_crash_effects (item_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_, err = crashCommand(t, db, effects, phase)
	t.Fatalf("crash point %s was not reached: %v", phase, err)
}

func TestCommandReceiptSurvivesAbruptProcessExit(t *testing.T) {
	for _, phase := range []string{"claim", "business", "receipt"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			effectsPath := filepath.Join(t.TempDir(), "effects.db")
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandCrashHelper$")
			command.Env = append(os.Environ(), "FLAME_TEST_COMMAND_CRASH="+phase, "FLAME_TEST_COMMAND_DB="+path, "FLAME_TEST_COMMAND_EFFECTS="+effectsPath)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
				t.Fatalf("abrupt exit: %v\n%s", err, output)
			}
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			effects := openCommandEffects(t, effectsPath)
			for range 2 {
				response, err := crashCommand(t, db, effects, "")
				if phase == "receipt" {
					if err != nil || response == nil || response.UserItemID != "item_crash" {
						t.Fatalf("durable receipt lost its identity: %+v, %v", response, err)
					}
				} else if !errors.Is(err, protocol.ErrIdempotencyInProgress) || response != nil {
					t.Fatalf("unresolved claim became a known outcome: %+v, %v", response, err)
				}
			}
			var executions int
			if err := effects.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM command_crash_effects`).Scan(&executions); err != nil {
				t.Fatal(err)
			}
			want := 1
			if phase == "claim" {
				want = 0
			}
			if executions != want {
				t.Fatalf("business executions = %d, want %d", executions, want)
			}
		})
	}
}

func openCommandEffects(t *testing.T, path string) *sql.DB {
	t.Helper()
	effects, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = effects.Close() })
	return effects
}
