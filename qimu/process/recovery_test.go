package process_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/process"
	"github.com/Tangerg/flame/qimu/runner"
	"github.com/Tangerg/flame/qimu/sqlite"
)

// A restart can replace the endpoint between two successful control calls.
// Each response here still comes from a real, independently owned runner.
type replaceAfterIdentity struct {
	process.Runner
	replacement process.Runner
}

func (r *replaceAfterIdentity) Identity(ctx context.Context) (runner.Identity, error) {
	identity, err := r.Runner.Identity(ctx)
	if err == nil && r.replacement != nil {
		r.Runner, r.replacement = r.replacement, nil
	}
	return identity, err
}

func recoveryRunner(t *testing.T, directory string) *runner.Client {
	t.Helper()
	server, err := runner.Open(runner.Config{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(directory, "socket")
	listener, err := server.Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-done
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	return runner.Dial(socket)
}

func TestRecoveryRequiresOriginalSealedNonCreationEvidence(t *testing.T) {
	for _, scenario := range []string{"same-runner", "replacement-runner", "replacement-after-identity"} {
		t.Run(scenario, func(t *testing.T) {
			directory, err := os.MkdirTemp("", "qimu-recovery-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(directory) })
			original := recoveryRunner(t, filepath.Join(directory, "original"))
			path := filepath.Join(directory, "state.sqlite")
			store, err := sqlite.Open(t.Context(), sqlite.Config{Path: path, Admission: qimu.NewAdmission()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := original.Fence(t.Context(), store.Epoch()); err != nil {
				t.Fatal(err)
			}
			identity, err := original.Identity(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, capability, err := echoContract()
			if err != nil {
				t.Fatal(err)
			}
			contract := qimu.ContractRef{Name: capability.Descriptor().Reference, Version: "1", Digest: qimu.Digest(capability.Identity().Digest()), Kind: qimu.ServiceKind, Mode: qimu.PortableMode}
			root := qimu.ExportRef{InstallationID: "original", Release: qimu.Hash([]byte("release")), UnitID: "backend", Export: "echo", Contract: contract}
			binding := qimu.BindingSnapshot{FormatVersion: 1, Roots: []qimu.ExportRef{root}, Units: []qimu.UnitBinding{{Source: root.Unit(), Configuration: qimu.Hash([]byte(`{}`)), Imports: []qimu.ImportDeclaration{}}}}
			attempt := sqlite.Attempt{ID: "approved-before-crash", Epoch: store.Epoch(), Source: root.Unit(), Binding: binding, Realm: identity.ID, Profile: qimu.MCPProcess}
			if err := store.RecordAttempt(t.Context(), attempt); err != nil {
				t.Fatal(err)
			}
			// The Host crashes after its durable approval and before Launch.
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = sqlite.Open(t.Context(), sqlite.Config{Path: path, Admission: qimu.NewAdmission()})
			if err != nil {
				t.Fatal(err)
			}
			var current process.Runner = original
			if scenario != "same-runner" {
				replacement := recoveryRunner(t, filepath.Join(directory, "replacement"))
				current = replacement
				if scenario == "replacement-after-identity" {
					current = &replaceAfterIdentity{Runner: original, replacement: replacement}
				}
			}
			source := &source{}
			driver, err := process.New(process.Config{Runner: current, Store: store, Source: source, CloseTimeout: time.Second, MaxFrameBytes: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			err = driver.Recover(t.Context())
			if scenario == "same-runner" && err != nil {
				t.Fatalf("original sealed runner could not prove non-creation: %v", err)
			}
			if scenario != "same-runner" && !errors.Is(err, runner.ErrUnknown) {
				t.Fatalf("a different authority converted missing state into proof: %v", err)
			}
			records, err := store.Attempts(t.Context())
			if err != nil || len(records) != 1 || records[0].Attempt.ID != attempt.ID {
				t.Fatalf("original attempt changed: %+v %v", records, err)
			}
			dependencies, err := store.ExecutionDependencies(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "same-runner" {
				if !records[0].Released || records[0].Proof == nil || records[0].Proof.Kind != "sealed-not-started" || records[0].Proof.Realm != identity.ID || len(dependencies) != 0 {
					t.Fatalf("original non-creation evidence was not settled: %+v %+v", records, dependencies)
				}
			} else if records[0].Released || records[0].Proof != nil || len(dependencies) != 1 || dependencies[0] != root.Unit().Dependency() {
				t.Fatalf("unknown execution lost its exact protection: %+v %+v", records, dependencies)
			}
			if source.programCalls.Load() != 0 {
				t.Fatal("recovery tried to recreate the approved attempt")
			}
			if _, err := original.Inspect(t.Context(), attempt.ID); !errors.Is(err, runner.ErrNotFound) {
				t.Fatalf("the original runner received a replacement creation: %v", err)
			}
		})
	}
}
