package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu"
)

func launchFixture(t *testing.T, id string, epoch uint64, directory string) LaunchRequest {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	return LaunchRequest{AttemptID: id, Epoch: epoch, Command: executable, ExecutableDigest: qimu.Hash(data), Configuration: qimu.Hash([]byte(directory)), Args: []string{"-test.run=^TestRunnerProcessHelper$"}, Env: append(os.Environ(), "QIMU_RUNNER_HELPER=echo", "QIMU_RUNNER_COUNT="+filepath.Join(directory, "created")), Directory: directory}
}

func serveFixture(t *testing.T) (*Server, *Client, context.Context) {
	t.Helper()
	// Unix-domain paths have an OS limit independent of an archive's path limit.
	directory, err := os.MkdirTemp("", "qimu-runner-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	server, err := Open(Config{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := server.Listen(filepath.Join(directory, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
	return server, Dial(filepath.Join(directory, "socket")), ctx
}

func TestAttemptStartsOnceAndAttachCarriesActualProcessIO(t *testing.T) {
	server, client, ctx := serveFixture(t)
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "convert-1", 1, t.TempDir())
	observation, err := client.Launch(ctx, request)
	if err != nil || observation.State != Running {
		t.Fatalf("launch: %+v %v", observation, err)
	}
	stream, err := client.Attach(ctx, request.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("exact binding\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stream).ReadString('\n')
	if err != nil || line != "exact binding\n" {
		t.Fatalf("process stream: %q %v", line, err)
	}
	if _, err := client.Attach(ctx, request.AttemptID); !errors.Is(err, ErrAttached) {
		t.Fatalf("second attachment: %v", err)
	}
	changed := request
	changed.Configuration = qimu.Hash([]byte("another configuration"))
	if _, err := client.Launch(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed attempt: %v", err)
	}
	closed, err := client.CloseAttempt(ctx, request.AttemptID)
	if err != nil || closed.State != Exited || closed.ProofID == "" {
		t.Fatalf("exit: %+v %v", closed, err)
	}
	_ = stream.Close()
	again, err := client.Launch(ctx, request)
	if err != nil || again.ProofID != closed.ProofID || again.State != Exited {
		t.Fatalf("old launch restarted: %+v %v", again, err)
	}
	count, err := os.ReadFile(filepath.Join(request.Directory, "created"))
	if err != nil || string(count) != "created\n" {
		t.Fatalf("creation count: %q %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(server.config.Directory, "attempts.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFenceSettlesOldCreationAndKeepsOriginalExitReachable(t *testing.T) {
	_, client, ctx := serveFixture(t)
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "old", 1, t.TempDir())
	if _, err := client.Launch(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := client.Fence(ctx, 2); err != nil {
		t.Fatal(err)
	}
	late := request
	late.AttemptID = "late-old"
	if _, err := client.Launch(ctx, late); !errors.Is(err, ErrFenced) {
		t.Fatalf("late old creation: %v", err)
	}
	if _, err := client.CloseAttempt(ctx, request.AttemptID); err != nil {
		t.Fatal(err)
	}
	current := request
	current.AttemptID = "current"
	current.Epoch = 2
	if _, err := client.Launch(ctx, current); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CloseAttempt(ctx, current.AttemptID); err != nil {
		t.Fatal(err)
	}
}

func TestFailedExecutableIsTerminalAndNeverStartsOnRetry(t *testing.T) {
	_, client, ctx := serveFixture(t)
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "bad", 1, t.TempDir())
	request.ExecutableDigest = qimu.Hash([]byte("not the executable"))
	first, err := client.Launch(ctx, request)
	if err == nil || first.State != Exited || first.ProofID == "" {
		t.Fatalf("bad executable: %+v %v", first, err)
	}
	second, err := client.Launch(ctx, request)
	if err != nil || second.ProofID != first.ProofID {
		t.Fatalf("retry changed failure: %+v %v", second, err)
	}
	if _, err := os.Stat(filepath.Join(request.Directory, "created")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected execution: %v", err)
	}
}

func TestSeparateRunnerSurvivesClientAndLosesNoAttemptOnRestart(t *testing.T) {
	directory, err := os.MkdirTemp("", "qimu-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	command := exec.Command(os.Args[0], "-test.run=^TestRunnerProcessHelper$")
	command.Env = append(os.Environ(), "QIMU_RUNNER_HELPER=daemon", "QIMU_RUNNER_DIRECTORY="+directory)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		_ = command.Process.Kill()
		waitErr := command.Wait()
		command.Process = nil
		t.Fatalf("daemon ready: %q %v %s", line, errors.Join(err, waitErr), diagnostics.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := Dial(filepath.Join(directory, "socket"))
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "retained-attempt", 1, t.TempDir())
	first, err := client.Launch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	// A new Host connection observes the original process, without receiving any
	// launch handle or replaying the effect.
	reconnected := Dial(filepath.Join(directory, "socket"))
	seen, err := reconnected.Inspect(ctx, request.AttemptID)
	if err != nil || seen.PID != first.PID || seen.RunnerID != first.RunnerID {
		t.Fatalf("reconnected observation: %+v %v", seen, err)
	}
	stream, err := reconnected.Attach(ctx, request.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("created\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(stream).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("runner did not crash")
	}
	command.Process = nil
	_ = stream.Close()
	server, err := Open(Config{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err := server.Fence(ctx, 2); err != nil {
		t.Fatal(err)
	}
	old, err := server.Launch(ctx, request)
	if err != nil || old.State != Unknown || old.PID != 0 || old.ProofID != "" {
		t.Fatalf("restart fabricated ownership: %+v %v", old, err)
	}
	if _, err := server.CloseAttempt(ctx, request.AttemptID); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown process was killed by stale identity: %v", err)
	}
	count, err := os.ReadFile(filepath.Join(request.Directory, "created"))
	if err != nil || string(count) != "created\n" {
		t.Fatalf("restart created another child: %q %v", count, err)
	}
	stale := request
	stale.AttemptID = "missing-old-attempt"
	if _, err := server.Launch(ctx, stale); !errors.Is(err, ErrFenced) {
		t.Fatalf("missing old attempt restarted: %v", err)
	}
}

func TestRunnerProcessHelper(t *testing.T) {
	switch os.Getenv("QIMU_RUNNER_HELPER") {
	case "echo":
		file, err := os.OpenFile(os.Getenv("QIMU_RUNNER_COUNT"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(31)
		}
		_, err = file.WriteString("created\n")
		if err != nil {
			os.Exit(32)
		}
		_ = file.Sync()
		_ = file.Close()
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		os.Exit(0)
	case "daemon":
		directory := os.Getenv("QIMU_RUNNER_DIRECTORY")
		server, err := Open(Config{Directory: directory})
		if err != nil {
			panic(err)
		}
		listener, err := server.Listen(filepath.Join(directory, "socket"))
		if err != nil {
			panic(err)
		}
		fmt.Println("ready")
		if err := server.Serve(context.Background(), listener); err != nil {
			panic(err)
		}
		os.Exit(0)
	}
}

func TestJournalContainsNoEnvironmentSecrets(t *testing.T) {
	server, client, ctx := serveFixture(t)
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "secret", 1, t.TempDir())
	request.CredentialRevision = "credential-1"
	request.Env = append(request.Env, "QIMU_TEST_PRIVATE_TOKEN=must-not-be-persisted")
	if _, err := client.Launch(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CloseAttempt(ctx, request.AttemptID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(server.config.Directory, "attempts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "must-not-be-persisted") || strings.Contains(string(data), "QIMU_TEST_PRIVATE_TOKEN") {
		t.Fatal("secret environment was persisted")
	}
	var value journal
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentDisconnectClosesOriginalProcess(t *testing.T) {
	_, client, ctx := serveFixture(t)
	if err := client.Fence(ctx, 1); err != nil {
		t.Fatal(err)
	}
	request := launchFixture(t, "disconnect", 1, t.TempDir())
	if _, err := client.Launch(ctx, request); err != nil {
		t.Fatal(err)
	}
	stream, err := client.Attach(ctx, request.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stream, "ready\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(stream).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	observation, err := client.CloseAttempt(ctx, request.AttemptID)
	if err != nil || observation.State != Exited || observation.ProofID == "" {
		t.Fatalf("disconnect did not settle original exit: %+v %v", observation, err)
	}
}
