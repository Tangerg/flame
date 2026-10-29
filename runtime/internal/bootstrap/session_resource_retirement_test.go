package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
)

type retirementFixture struct {
	db          *persistencefixture.Stores
	stores      sessionStores
	coordinator *sessions.Coordinator
	gate        *ownership.Gate
	resources   *retirementResources
	workspace   string
	root        string
}

type retirementResources struct {
	*workspaceadapter.Checkpoints
	isolator                *workspaceadapter.Isolator
	dropErr, discardErr     error
	afterDrop, afterDiscard func()
}

func (r *retirementResources) DropSession(id string) error {
	if r.dropErr != nil {
		return r.dropErr
	}
	if err := r.Checkpoints.DropSession(id); err != nil {
		return err
	}
	if r.afterDrop != nil {
		r.afterDrop()
	}
	return nil
}
func (r *retirementResources) Discard(id string) error {
	if r.discardErr != nil {
		return r.discardErr
	}
	if err := r.isolator.Discard(id); err != nil {
		return err
	}
	if r.afterDiscard != nil {
		r.afterDiscard()
	}
	return nil
}

type retirementLedger struct {
	*persistence.ResourceRetirementStore
	beforeComplete func()
}

func (s retirementLedger) Complete(ctx context.Context, value sessions.ResourceRetirement) error {
	if s.beforeComplete != nil {
		s.beforeComplete()
	}
	return s.ResourceRetirementStore.Complete(ctx, value)
}

type retirementWrites struct {
	*persistence.SessionStores
	afterDelete func()
}

func (s retirementWrites) ApplyDelete(ctx context.Context, value sessions.DeletePlan) error {
	if err := s.SessionStores.ApplyDelete(ctx, value); err != nil {
		return err
	}
	if s.afterDelete != nil {
		s.afterDelete()
	}
	return nil
}

type retirementTransient struct{}

func (retirementTransient) QuiesceSession(string) error                        { return nil }
func (retirementTransient) QuiesceWorkspace(string) error                      { return nil }
func (retirementTransient) ForgetSession(string)                               {}
func (retirementTransient) ForgetSessionContext(string)                        {}
func (retirementTransient) ForgetWorkspace(string)                             {}
func (retirementTransient) Release(context.Context, runsapp.ExecutorRef) error { return nil }

func openRetirementFixture(t *testing.T, root, cut string) *retirementFixture {
	t.Helper()
	db, err := persistencefixture.Open(t, t.Context(), filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	stores, _, _ := newWriteSetStores(t, db.Bundle)
	gate, err := ownership.NewGate(ownership.GateConfig{
		ResolveTree: filesystem.CanonicalPath, RetirementPending: db.ResourceRetirements.Pending,
	})
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := workspaceadapter.NewIsolator(root, filepath.Join(db.DataDirectory, "sandbox"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := isolator.Close(); err != nil {
			t.Error(err)
		}
	})
	resources := &retirementResources{Checkpoints: workspaceadapter.NewCheckpoints(filepath.Join(db.DataDirectory, "checkpoints")), isolator: isolator}
	ledger := retirementLedger{ResourceRetirementStore: db.ResourceRetirements}
	writes := retirementWrites{SessionStores: stores.SessionStores}
	crash := func() { os.Exit(77) }
	switch cut {
	case "commit":
		writes.afterDelete = crash
	case "checkpoints":
		resources.afterDrop = crash
	case "sandbox":
		resources.afterDiscard = crash
	case "completion":
		ledger.beforeComplete = crash
	}
	coordinator, err := sessions.New(sessions.Dependencies{
		Sessions: stores.sessions, Interrupts: stores.interrupts, Transcript: stores.transcript, Runs: stores.runs,
		Snapshots: stores.SessionStores, MaterialSnapshots: stores.SessionStores, Writes: writes,
		Plan:           sessions.PlanServices{Boundaries: stores.plan, Replacements: mustPlanCoordinator(sessions.PlanDependencies{Store: stores.plan, Now: time.Now})},
		TransientState: retirementTransient{}, ExecutionReleaser: retirementTransient{},
		Paths: workspaceadapter.Resolver{}, Models: catalog.Selections{}, DefaultModelSelection: testsupport.DefaultModelSelection(),
		Checkpoints: resources, Sandbox: resources, Goals: goals.NewSessionMutations(),
		Mutations: db.WorkspaceMutations, Retirements: ledger, Admissions: gate,
		NewID: newSessionID, NewRunID: newRunID, NewItemID: newItemID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &retirementFixture{db: db, stores: stores, coordinator: coordinator, gate: gate, resources: resources, workspace: filepath.Join(root, "project"), root: root}
}

func (f *retirementFixture) seed(t *testing.T) sessions.PortableSnapshot {
	t.Helper()
	if err := os.MkdirAll(f.workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", f.workspace, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	original := bootstrapSession("ses_retirement", "original", f.workspace)
	insertBootstrapSession(t, f.stores.sessions, original)
	if err := f.stores.runs.Restore(t.Context(), restoredRun(original.ID(), "run_boundary", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := f.stores.history.Seed(t.Context(), original.ID(), []chat.Message{chat.NewUserMessage(chat.NewTextPart("original history"))}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.workspace, "material.txt"), []byte("boundary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.resources.Snapshot(t.Context(), original.ID(), f.workspace, "run_boundary"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.stores.ReadSnapshot(t.Context(), original.ID())
	if err != nil {
		t.Fatal(err)
	}
	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	portable.Session.Title = "replacement"
	return portable
}

func (f *retirementFixture) assertOriginalBoundary(t *testing.T) {
	t.Helper()
	current, err := f.stores.sessions.Get(t.Context(), "ses_retirement")
	if err != nil || current.Title() != "original" {
		t.Fatalf("original Session = %+v, %v", current.Snapshot(), err)
	}
	history, err := f.stores.history.Read(t.Context(), current.ID())
	if err != nil || len(history) != 1 {
		t.Fatalf("original history = %+v, %v", history, err)
	}
	if err := os.WriteFile(filepath.Join(f.workspace, "material.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.resources.Restore(t.Context(), current.ID(), f.workspace, "run_boundary"); err != nil {
		t.Fatalf("original file boundary lost: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.workspace, "material.txt")); err != nil || string(body) != "boundary\n" {
		t.Fatalf("restored file = %q, %v", body, err)
	}
	if pending, err := f.stores.retirements.Pending(t.Context(), current.ID()); err != nil || pending {
		t.Fatalf("refused transaction authorized cleanup: %t, %v", pending, err)
	}
}

func TestSessionWriteRefusalPreservesRealFileCheckpoints(t *testing.T) {
	for _, operation := range []string{"delete", "restore", "conflicting run", "delete retirement insert", "restore retirement insert"} {
		t.Run(operation, func(t *testing.T) {
			f := openRetirementFixture(t, t.TempDir(), "")
			portable := f.seed(t)
			var err error
			if operation == "delete retirement insert" || operation == "restore retirement insert" {
				if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER refuse_retirement BEFORE INSERT ON committed_session_resource_retirements BEGIN SELECT RAISE(ABORT, 'retirement refused'); END`); err != nil {
					t.Fatal(err)
				}
			}
			switch operation {
			case "delete retirement insert":
				err = f.coordinator.DeleteSession(t.Context(), "ses_retirement")
			case "restore retirement insert":
				_, err = f.coordinator.RestorePortableSession(t.Context(), portable)
			case "delete":
				if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER refuse_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'delete refused'); END`); err != nil {
					t.Fatal(err)
				}
				err = f.coordinator.DeleteSession(t.Context(), "ses_retirement")
			case "restore":
				if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER refuse_restore BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'restore refused'); END`); err != nil {
					t.Fatal(err)
				}
				_, err = f.coordinator.RestorePortableSession(t.Context(), portable)
			case "conflicting run":
				insertBootstrapSession(t, f.stores.sessions, bootstrapSession("ses_other", "other", f.workspace))
				if err := f.stores.runs.Restore(t.Context(), restoredRun("ses_other", "run_conflict", time.Now().UTC())); err != nil {
					t.Fatal(err)
				}
				portable.Runs[0].ID = "run_conflict"
				_, err = f.coordinator.RestorePortableSession(t.Context(), portable)
				if !errors.Is(err, run.ErrIdentityConflict) {
					t.Fatalf("restore conflict = %v", err)
				}
			}
			if err == nil {
				t.Fatal("write unexpectedly succeeded")
			}
			f.assertOriginalBoundary(t)
		})
	}
}

func TestCommittedResourceCleanupFailureFencesReplacement(t *testing.T) {
	for _, operation := range []string{"delete", "restore"} {
		for _, failing := range []string{"checkpoints", "sandbox"} {
			t.Run(operation+"/"+failing, func(t *testing.T) {
				f := openRetirementFixture(t, t.TempDir(), "")
				portable := f.seed(t)
				refusal := errors.New("owned resource cleanup refused")
				if failing == "checkpoints" {
					f.resources.dropErr = refusal
				} else {
					f.resources.discardErr = refusal
				}
				if operation == "delete" {
					if err := f.coordinator.DeleteSession(t.Context(), "ses_retirement"); err != nil {
						t.Fatalf("committed delete reported unexecuted: %v", err)
					}
					if _, err := f.stores.sessions.Get(t.Context(), "ses_retirement"); !errors.Is(err, session.ErrNotFound) {
						t.Fatalf("deleted Session = %v", err)
					}
				} else {
					view, err := f.coordinator.RestorePortableSession(t.Context(), portable)
					if err != nil {
						t.Fatalf("committed restore reported unexecuted: %v", err)
					}
					if view.Title != "replacement" {
						t.Fatalf("restore result = %+v", view)
					}
				}
				pending, found, err := f.stores.retirements.Lookup(t.Context(), "ses_retirement")
				if err != nil || !found {
					t.Fatalf("committed responsibility = %v, %v", found, err)
				}
				wrong, err := sessions.NewDeletePlan("ses_retirement")
				if err != nil {
					t.Fatal(err)
				}
				if err := f.stores.retirements.Complete(t.Context(), wrong.Retirement()); err != nil {
					t.Fatal(err)
				}
				if actual, found, err := f.stores.retirements.Lookup(t.Context(), "ses_retirement"); err != nil || !found || actual != pending {
					t.Fatalf("wrong generation cleared responsibility: %+v, %v, %v", actual, found, err)
				}
				if release, ok, err := f.gate.AcquireSession(t.Context(), "ses_retirement"); err != nil || ok {
					if release != nil {
						release()
					}
					t.Fatalf("Session crossed pending cleanup: %t, %v", ok, err)
				}
				if admission, ok, err := f.gate.AcquireRun(t.Context(), "ses_retirement", f.workspace); err != nil || ok {
					admission.Release()
					t.Fatalf("Run crossed pending cleanup: %t, %v", ok, err)
				}
				if err := f.stores.runs.Admit(t.Context(), testsupport.RunDraft(run.Draft{SessionID: "ses_retirement", RunID: "run_new", SegmentID: "seg_new", CreatedAt: time.Now().UTC()})); !errors.Is(err, run.ErrSessionBusy) {
					t.Fatalf("durable admission crossed pending cleanup: %v", err)
				}
				if _, err := f.coordinator.RestorePortableSession(t.Context(), portable); !errors.Is(err, sessions.ErrSessionBusy) {
					t.Fatalf("replacement crossed pending cleanup: %v", err)
				}
				if err := f.coordinator.RecoverRetiredResources(t.Context()); !errors.Is(err, refusal) {
					t.Fatalf("recovery erased cleanup refusal: %v", err)
				}
				f.resources.dropErr, f.resources.discardErr = nil, nil
				if err := f.coordinator.RecoverRetiredResources(t.Context()); err != nil {
					t.Fatal(err)
				}
				if pending, err := f.stores.retirements.Pending(t.Context(), "ses_retirement"); err != nil || pending {
					t.Fatalf("completed responsibility = %v, %v", pending, err)
				}
				if err := f.resources.Restore(t.Context(), "ses_retirement", f.workspace, "run_boundary"); !errors.Is(err, sessions.ErrCheckpointUnavailable) {
					t.Fatalf("old checkpoint survived retirement: %v", err)
				}
				if release, ok, err := f.gate.AcquireSession(t.Context(), "ses_retirement"); err != nil || !ok {
					t.Fatalf("Session remained fenced: %t, %v", ok, err)
				} else {
					release()
				}
			})
		}
	}
}

func TestCommittedResourceRetirementCrashRecovery(t *testing.T) {
	if cut := os.Getenv("FLAME_RETIREMENT_CRASH_CUT"); cut != "" {
		f := openRetirementFixture(t, os.Getenv("FLAME_RETIREMENT_CRASH_ROOT"), cut)
		f.seed(t)
		scratch, err := f.resources.isolator.Workspace(t.Context(), "ses_retirement", f.workspace)
		if errors.Is(err, sandbox.ErrUnavailable) {
			os.Exit(78)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.root, "scratch-path"), []byte(scratch), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := f.coordinator.DeleteSession(t.Context(), "ses_retirement"); err != nil {
			t.Fatal(err)
		}
		t.Fatal("crash cut was not reached")
	}
	for _, cut := range []string{"commit", "checkpoints", "sandbox", "completion"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			child := exec.Command(os.Args[0], "-test.run=^TestCommittedResourceRetirementCrashRecovery$", "-test.count=1")
			child.Env = append(os.Environ(), "FLAME_RETIREMENT_CRASH_CUT="+cut, "FLAME_RETIREMENT_CRASH_ROOT="+root)
			output, err := child.CombinedOutput()
			var exited *exec.ExitError
			if errors.As(err, &exited) && exited.ExitCode() == 78 {
				t.Skip("host has no sandbox backend")
			}
			if !errors.As(err, &exited) || exited.ExitCode() != 77 {
				t.Fatalf("crash cut %s: %v\n%s", cut, err, output)
			}
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: filepath.Join(root, "data")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := stores.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err := stores.Sessions.Get(t.Context(), "ses_retirement"); !errors.Is(err, session.ErrNotFound) {
				t.Fatalf("history was not committed before crash: %v", err)
			}
			if pending, err := stores.ResourceRetirements.Pending(t.Context(), "ses_retirement"); err != nil || !pending {
				t.Fatalf("crash lost responsibility: %t, %v", pending, err)
			}
			host, _ := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, newReplyStub("recovered")), filepath.Join(root, "project"))
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			if pending, err := stores.ResourceRetirements.Pending(t.Context(), "ses_retirement"); err != nil || pending {
				t.Fatalf("startup retained responsibility: %t, %v", pending, err)
			}
			checkpoints := workspaceadapter.NewCheckpoints(filepath.Join(stores.DataDirectory, "checkpoints"))
			if err := checkpoints.Restore(t.Context(), "ses_retirement", filepath.Join(root, "project"), "run_boundary"); !errors.Is(err, sessions.ErrCheckpointUnavailable) {
				t.Fatalf("old checkpoint survived startup: %v", err)
			}
			scratch, err := os.ReadFile(filepath.Join(root, "scratch-path"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(string(scratch)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("old scratch survived restart: %v", err)
			}
			if body, err := os.ReadFile(filepath.Join(root, "project", "material.txt")); err != nil || string(body) != "boundary\n" {
				t.Fatalf("cleanup changed source workspace: %q, %v", body, err)
			}
		})
	}
}
