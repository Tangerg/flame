package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/skills"
	modernsqlite "modernc.org/sqlite"
)

const (
	skillMoveCrashVariable = "FLAME_TEST_SKILL_MOVE_CRASH"
	skillMoveCutVariable   = "FLAME_TEST_SKILL_MOVE_CUT"
	skillMoveBeforeRename  = "before_rename"
	skillMoveBeforeReceipt = "before_receipt"
	skillMoveCrashName     = "original-skill"
	skillMoveCrashKey      = "original-skill-move"
	skillMoveCrashDocument = "---\nname: original-skill\ndescription: Keep all authored resources.\n---\nFollow the original instructions.\n"
	skillMoveCrashResource = "original skill resource"
)

func TestSkillMoveCrashHelper(t *testing.T) {
	method := delivery.Name(os.Getenv(skillMoveCrashVariable))
	if method == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	point := os.Getenv(skillMoveCutVariable)
	if err := modernsqlite.RegisterScalarFunction("test_skill_lifecycle_crash", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			if point == skillMoveBeforeRename {
				return nil, os.WriteFile(filepath.Join(home, "skills", "_archive"), nil, 0600)
			}
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	host, _ := openProtocolRuntime(t, newReplyStub("unused"))
	defer host.Close()
	cut := "BEFORE UPDATE OF payload ON idempotency_records"
	if point == skillMoveBeforeRename {
		cut = "AFTER INSERT ON pending_skill_move"
	}
	questionCrashDatabase(t, home, `CREATE TRIGGER interrupt_skill_move_settlement
		`+cut+` BEGIN SELECT test_skill_lifecycle_crash(); END`)
	result := callSkillMove(t.Context(), host.Endpoint(), method, skillMoveCrashKey)
	if point == skillMoveBeforeRename && result.Failure != nil && errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
		os.Exit(commandCrashExit)
	}
	t.Fatalf("receipt cutpoint was not reached: %v", result.Failure)
}

func TestSkillLifecycleAcknowledgementSurvivesAbruptProcessExit(t *testing.T) {
	for _, test := range []struct {
		name            string
		method, reverse delivery.Name
	}{
		{"archive", delivery.SkillsLibraryArchive, delivery.SkillsLibraryRestore},
		{"restore", delivery.SkillsLibraryRestore, delivery.SkillsLibraryArchive},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, destination := crashSkillMove(t, test.method, skillMoveBeforeReceipt)
			endpoint := restartAfterSkillMove(t)
			if result := callSkillMove(t.Context(), endpoint, test.reverse, "reverse-skill-move"); result.Failure != nil {
				t.Fatalf("later move before replay: %v", result.Failure)
			}
			if result := callSkillMove(t.Context(), endpoint, test.method, skillMoveCrashKey); result.Failure != nil || result.Value != (struct{}{}) {
				t.Fatalf("original %s acknowledgement: %+v, %v", test.method, result.Value, result.Failure)
			}
			assertSkillMoveLocation(t, source, destination, skillMoveCrashResource)
		})
	}
}

func TestSkillMoveReceiptRefusesAnIdenticallyNamedReplacement(t *testing.T) {
	source, destination := crashSkillMove(t, delivery.SkillsLibraryArchive, skillMoveBeforeReceipt)
	retained := filepath.Join(os.Getenv("FLAME_HOME"), "retained-original")
	if err := os.Rename(destination, retained); err != nil {
		t.Fatal(err)
	}
	const otherResource = "different authored resource"
	writeSkillMoveFixture(t, destination, otherResource)
	endpoint := restartAfterSkillMove(t)
	for _, key := range []string{skillMoveCrashKey, "new-skill-archive"} {
		result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryArchive, key)
		if !errors.Is(result.Failure, protocol.ErrSkillNotFound) {
			t.Fatalf("archive acknowledges the replacement for %q: %v", key, result.Failure)
		}
	}
	assertSkillMoveLocation(t, destination, source, otherResource)
	if got, err := os.ReadFile(filepath.Join(retained, "resources", "reference.txt")); err != nil || string(got) != skillMoveCrashResource {
		t.Fatalf("retained original resource = %q, %v", got, err)
	}
}

func TestSkillMoveRecoversOnlyAnUnchangedSourceAfterAbruptProcessExit(t *testing.T) {
	for _, test := range []struct {
		name   string
		change bool
	}{
		{name: "unchanged"},
		{name: "edited", change: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, destination := crashSkillMove(t, delivery.SkillsLibraryArchive, skillMoveBeforeRename)
			if test.change {
				if err := os.WriteFile(filepath.Join(source, skills.SkillFile), []byte(skillMoveCrashDocument+"New revision.\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			endpoint := restartAfterSkillMove(t)
			result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryArchive, skillMoveCrashKey)
			if test.change {
				if !errors.Is(result.Failure, protocol.ErrSkillNotFound) {
					t.Fatalf("edited source acknowledged: %v", result.Failure)
				}
				assertSkillMoveLocation(t, source, destination, skillMoveCrashResource)
				return
			}
			if result.Failure != nil {
				t.Fatalf("unchanged source did not resume: %v", result.Failure)
			}
			assertSkillMoveLocation(t, destination, source, skillMoveCrashResource)
		})
	}
}

func TestSkillMoveRequiresCallerIdentity(t *testing.T) {
	for _, method := range []delivery.Name{delivery.SkillsLibraryArchive, delivery.SkillsLibraryRestore} {
		t.Run(method.String(), func(t *testing.T) {
			home := testsupport.TempDirectory(t)
			t.Setenv("FLAME_HOME", home)
			source := filepath.Join(home, "skills", skillMoveCrashName)
			destination := filepath.Join(home, "skills", "_archive", skillMoveCrashName)
			if method == delivery.SkillsLibraryRestore {
				source, destination = destination, source
			}
			writeSkillMoveFixture(t, source, skillMoveCrashResource)
			result := callSkillMove(t.Context(), restartAfterSkillMove(t), method, "")
			if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
				t.Errorf("%s without a key = %+v, %v; want invalid params", method, result.Value, result.Failure)
			}
			assertSkillMoveLocation(t, source, destination, skillMoveCrashResource)
		})
	}
}

func TestSkillMoveSettlesOnlyItsOriginalRequest(t *testing.T) {
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	source := filepath.Join(home, "skills", skillMoveCrashName)
	destination := filepath.Join(home, "skills", "_archive", skillMoveCrashName)
	writeSkillMoveFixture(t, source, skillMoveCrashResource)
	endpoint := restartAfterSkillMove(t)
	allowReceipt := refuseCommandReceipt(t, home)
	result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryArchive, skillMoveCrashKey)
	allowReceipt()
	if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("refused archive receipt: %v", result.Failure)
	}
	assertSkillMoveLocation(t, destination, source, skillMoveCrashResource)
	const restoreKey = "restore-after-refusal"
	if result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryRestore, restoreKey); result.Failure == nil || !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("a different key advanced the unsettled move: %+v, %v", result.Value, result.Failure)
	}
	assertSkillMoveLocation(t, destination, source, skillMoveCrashResource)
	if result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryArchive, skillMoveCrashKey); result.Failure != nil {
		t.Fatalf("original archive receipt: %v", result.Failure)
	}
	if result := callSkillMove(t.Context(), endpoint, delivery.SkillsLibraryRestore, restoreKey); result.Failure != nil {
		t.Fatalf("later restore with its original key: %v", result.Failure)
	}
	assertSkillMoveLocation(t, source, destination, skillMoveCrashResource)
}

func crashSkillMove(t *testing.T, method delivery.Name, point string) (source, destination string) {
	t.Helper()
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	source = filepath.Join(home, "skills", skillMoveCrashName)
	destination = filepath.Join(home, "skills", "_archive", skillMoveCrashName)
	if method == delivery.SkillsLibraryRestore {
		source, destination = destination, source
	}
	writeSkillMoveFixture(t, source, skillMoveCrashResource)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSkillMoveCrashHelper$")
	command.Env = append(os.Environ(), skillMoveCrashVariable+"="+method.String(), skillMoveCutVariable+"="+point)
	output, err := command.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("abrupt exit: %v\n%s", err, output)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER interrupt_skill_move_settlement")
	if point == skillMoveBeforeRename {
		if err := os.Remove(filepath.Dir(destination)); err != nil {
			t.Fatal(err)
		}
		assertSkillMoveLocation(t, source, destination, skillMoveCrashResource)
	} else {
		assertSkillMoveLocation(t, destination, source, skillMoveCrashResource)
	}
	return source, destination
}

func restartAfterSkillMove(t *testing.T) *delivery.Endpoint {
	t.Helper()
	host, _ := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	return host.Endpoint()
}

func writeSkillMoveFixture(t *testing.T, directory, resource string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(directory, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, skills.SkillFile), []byte(skillMoveCrashDocument), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "resources", "reference.txt"), []byte(resource), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertSkillMoveLocation(t *testing.T, present, absent, resource string) {
	t.Helper()
	if content, err := os.ReadFile(filepath.Join(present, "resources", "reference.txt")); err != nil || string(content) != resource {
		t.Fatalf("resource at %s: %q, %v", present, content, err)
	}
	if _, err := os.Lstat(absent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous directory still exists at %s: %v", absent, err)
	}
}

func callSkillMove(ctx context.Context, endpoint *delivery.Endpoint, method delivery.Name, key string) protocol.OperationResult {
	return endpoint.Invoke(ctx, method, protocol.SkillNameRequest{Name: skillMoveCrashName}, protocol.CallMetadata{IdempotencyKey: key})
}
