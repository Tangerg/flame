//go:build darwin

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/sandbox/internal/contract"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
)

func TestNativeExecutionConfinesEffectsAndReplaysTheOriginalOutcomeAfterReopen(t *testing.T) {
	data, home := t.TempDir(), t.TempDir()
	secret := filepath.Join(home, "secret")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(secret, []byte("private-home-content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_SANDBOX_SECRET", "parent-credential")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	input := rpc.ExecuteInput{Script: "printf inside > private-file; test ! -r " + quote(secret) + " || exit 91; if printf changed > " + quote(outside) + "; then exit 92; fi; printf '%s' \"${FLAME_SANDBOX_SECRET-unset}\"; exit 7", TimeoutMillis: 3000}
	encoded, _ := jsonvalue.Encode(input)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "sandbox.db")
	db, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	homeRead := func(context.Context) (string, error) { calls++; return home, nil }
	runner, err := newRunner(db, filepath.Join(data, "scratch"), homeRead)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.execute(t.Context(), contracts.Execute.Name(), "original", input, encoded)
	if err != nil || result.Status != sandbox.Completed || result.ExitCode != 7 || result.Stdout.Content != "unset" {
		t.Fatalf("native isolation lost its outcome: %+v %v", result, err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "original" {
		t.Fatalf("sandbox wrote outside its scratch root: %q %v", content, err)
	}
	if entries, err := os.ReadDir(runner.scratch); err != nil || len(entries) != 0 {
		t.Fatalf("known execution retained its scratch files: %v %v", entries, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner, err = newRunner(db, filepath.Join(data, "scratch"), homeRead)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := runner.execute(t.Context(), contracts.Execute.Name(), "original", input, encoded)
	if err != nil || !reflect.DeepEqual(replay, result) || calls != 1 {
		t.Fatalf("replay repeated execution: %+v %v calls=%d", replay, err, calls)
	}
	changed := rpc.ExecuteInput{Script: "exit 0", TimeoutMillis: 3000}
	changedJSON, _ := jsonvalue.Encode(changed)
	if _, err := runner.execute(t.Context(), contracts.Execute.Name(), "original", changed, changedJSON); !errors.Is(err, portable.ErrInvocationConflict) {
		t.Fatalf("accepted identity changed script: %v", err)
	}
	binary := rpc.ExecuteInput{Script: "printf '\\377'", TimeoutMillis: 3000}
	binaryJSON, _ := jsonvalue.Encode(binary)
	output, err := runner.execute(t.Context(), contracts.Execute.Name(), "binary", binary, binaryJSON)
	raw, decodeErr := base64.StdEncoding.DecodeString(output.Stdout.Content)
	if err != nil || decodeErr != nil || output.Stdout.Encoding != sandbox.Base64 || string(raw) != "\xff" {
		t.Fatalf("binary stdout was replaced: %+v %v %v", output, err, decodeErr)
	}
	deadline := rpc.ExecuteInput{Script: "sleep 10 & wait", TimeoutMillis: 100}
	deadlineJSON, _ := jsonvalue.Encode(deadline)
	timed, err := runner.execute(t.Context(), contracts.Execute.Name(), "deadline", deadline, deadlineJSON)
	if err != nil || timed.Status != sandbox.TimedOut || timed.DurationMillis > 3000 {
		t.Fatalf("timeout did not terminate the command group: %+v %v", timed, err)
	}
}

func TestUnknownNativeCommandCannotAcquireANewHostReadAfterRestart(t *testing.T) {
	data := t.TempDir()
	path := filepath.Join(data, "sandbox.db")
	db, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("core observation unavailable")
	calls := 0
	read := func(context.Context) (string, error) { calls++; return "", cause }
	runner, _ := newRunner(db, filepath.Join(data, "scratch"), read)
	contracts, _ := rpc.NewContracts()
	input := rpc.ExecuteInput{Script: "printf should-not-run", TimeoutMillis: 1000}
	encoded, _ := jsonvalue.Encode(input)
	if _, err := runner.execute(t.Context(), contracts.Execute.Name(), "unknown", input, encoded); !errors.Is(err, cause) {
		t.Fatalf("unexpected failure became a refusal: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner, _ = newRunner(db, filepath.Join(data, "scratch"), read)
	if _, err := runner.execute(t.Context(), contracts.Execute.Name(), "unknown", input, encoded); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) || calls != 1 {
		t.Fatalf("unknown admission repeated work: %v calls=%d", err, calls)
	}
	invalid := rpc.ExecuteInput{Script: "printf before\x00after", TimeoutMillis: 1000}
	invalidJSON, _ := jsonvalue.Encode(invalid)
	for range 2 {
		_, err := runner.execute(t.Context(), contracts.Execute.Name(), "invalid", invalid, invalidJSON)
		refusal, known := errors.AsType[*portable.Refusal](err)
		if !known || refusal.Code != "invalid_program" || calls != 1 {
			t.Fatalf("local refusal did not settle before host access: %v", err)
		}
	}
}
