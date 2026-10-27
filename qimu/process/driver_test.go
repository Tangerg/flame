package process_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/process"
	"github.com/Tangerg/flame/qimu/runner"
	"github.com/Tangerg/flame/qimu/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func echoContract() (*mcp.Method[string, string], *portable.Capability, error) {
	method, err := mcp.NewMethod[string, string](mcp.MethodConfig{Name: "document.echo", Kind: portable.Query, Target: "document", Description: "Read the exact document text."})
	if err != nil {
		return nil, nil, err
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "document.echo", Composition: portable.Selection, Scope: "document"}, method.Descriptor())
	return method, contract, err
}

type activation struct{ directory string }

func (a activation) Start() error {
	return os.WriteFile(filepath.Join(a.directory, "activated"), []byte("once"), 0600)
}
func (activation) Close() error { return nil }

func TestMCPProcessHelper(t *testing.T) {
	directory := os.Getenv("QIMU_PROCESS_HELPER")
	if directory == "" {
		return
	}
	err := func() error {
		method, contract, err := echoContract()
		if err != nil {
			return err
		}
		server := sdk.NewServer(&sdk.Implementation{Name: "document-processor", Version: "1"}, nil)
		backend, err := mcp.NewBackend(server)
		if err != nil {
			return err
		}
		defer backend.Close()
		if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command mcp.Command[string]) (string, error) {
			if _, err := os.Stat(filepath.Join(directory, "activated")); err != nil {
				return "", errors.New("business executed before activation")
			}
			return command.Input, nil
		}); err != nil {
			return err
		}
		if err := backend.BindCapability("echo", contract); err != nil {
			return err
		}
		if err := backend.OnSetup(func(ctx context.Context, _ mcp.Setup) (io.Closer, error) {
			if err := os.WriteFile(filepath.Join(directory, "prepared"), []byte("once"), 0600); err != nil {
				return nil, err
			}
			if os.Getenv("QIMU_PROCESS_WAIT") == "1" {
				<-ctx.Done()
				return nil, context.Cause(ctx)
			}
			return activation{directory}, nil
		}); err != nil {
			return err
		}
		if err := backend.Publish(); err != nil {
			return err
		}
		return server.Run(context.Background(), &sdk.StdioTransport{})
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type source struct {
	program      process.Program
	contract     *portable.Capability
	programCalls atomic.Int32
}

func (s *source) Program(context.Context, qimu.UnitBinding) (process.Program, error) {
	s.programCalls.Add(1)
	return s.program, nil
}

func (s *source) Contract(_ context.Context, ref qimu.ContractRef) (*portable.Capability, error) {
	if ref.Digest != qimu.Digest(s.contract.Identity().Digest()) {
		return nil, qimu.ErrContractMismatch
	}
	return s.contract, nil
}

type environment struct {
	ctx       context.Context
	store     *sqlite.Store
	runner    *runner.Client
	host      *qimu.Host
	source    *source
	ref       qimu.ExportRef
	directory string
}

func newEnvironment(t *testing.T, wait bool) *environment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	directory, err := os.MkdirTemp("", "qimu-process-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	server, err := runner.Open(runner.Config{Directory: filepath.Join(directory, "runner")})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := server.Listen(filepath.Join(directory, "runner", "socket"))
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-served
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	client := runner.Dial(filepath.Join(directory, "runner", "socket"))
	admission := qimu.NewAdmission()
	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(directory, "state.sqlite"), Admission: admission})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := client.Fence(ctx, store.Epoch()); err != nil {
		t.Fatal(err)
	}
	_, capability, err := echoContract()
	if err != nil {
		t.Fatal(err)
	}
	ref := qimu.ContractRef{Name: capability.Descriptor().Reference, Version: "1", Digest: qimu.Digest(capability.Identity().Digest()), Kind: qimu.ServiceKind, Mode: qimu.PortableMode}
	contract, err := qimu.BindContract[*mcp.CapabilityBinding](ref, capability.Identity().JSON())
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	source := &source{contract: capability, program: process.Program{Command: executable, ExecutableDigest: qimu.Hash(data), Args: []string{"-test.run=^TestMCPProcessHelper$"}, Env: append(os.Environ(), "QIMU_PROCESS_HELPER="+directory), Directory: directory}}
	if wait {
		source.program.Env = append(source.program.Env, "QIMU_PROCESS_WAIT=1")
	}
	driver, err := process.New(process.Config{Runner: client, Store: store, Source: source, CloseTimeout: 3 * time.Second, MaxFrameBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	host, err := qimu.NewHost(ctx, qimu.HostConfig{Admission: admission, Epoch: store.Epoch(), Drivers: []qimu.Driver{driver}, Installations: store, ExecutionProtection: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if outcome, err := host.Close(closeCtx); err != nil || !outcome.Released {
			t.Errorf("host release: %+v %v", outcome, err)
		}
	})
	if err := host.Register(contract); err != nil {
		t.Fatal(err)
	}
	plugin, err := qimu.Define(qimu.Definition{Revision: qimu.Hash([]byte("echo release")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "backend", Profile: qimu.MCPProcess, Exports: []qimu.ExportDeclaration{{Name: "echo", Contract: ref}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Install(ctx, qimu.InstallationConfig{ID: "echo", Plugin: plugin, Configuration: []byte(`{}`), Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return &environment{ctx: ctx, store: store, runner: client, host: host, source: source, ref: qimu.ExportRef{InstallationID: "echo", Release: plugin.Revision(), UnitID: "backend", Export: "echo", Contract: ref}, directory: directory}
}

func TestApprovedProcessUsesPublicContractAndReleasesOriginalAttempt(t *testing.T) {
	e := newEnvironment(t, false)
	binding, err := e.host.Activate(e.ctx, e.ref)
	if err != nil {
		t.Fatal(err)
	}
	owner := qimu.NewLifetime(e.ctx)
	scope, err := e.host.Admission().OpenScope(e.ctx, "document")
	if err != nil {
		t.Fatal(err)
	}
	use, err := e.host.AcquireBinding(e.ctx, binding, qimu.UseConfig{Owner: owner, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode("document exact text")
	if err != nil {
		t.Fatal(err)
	}
	err = qimu.BorrowExport[*mcp.CapabilityBinding](e.ctx, use, e.ref, func(ctx context.Context, capability *mcp.CapabilityBinding) error {
		method, err := capability.Method("document.echo")
		if err != nil {
			return err
		}
		result, err := method.Invoke(ctx, "", input)
		if err == nil && result != input {
			return fmt.Errorf("reply changed: %s", result.String())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := use.Close(e.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Close(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.host.SetEnabled(e.ctx, "echo", false); err != nil {
		t.Fatal(err)
	}
	if err := e.host.WaitRetired(e.ctx, "echo"); err != nil {
		t.Fatal(err)
	}
	attempts, err := e.store.Attempts(e.ctx)
	if err != nil || len(attempts) != 1 || !attempts[0].Released || attempts[0].Proof == nil {
		t.Fatalf("original attempt release: %+v %v", attempts, err)
	}
	observed, err := e.runner.Inspect(e.ctx, attempts[0].Attempt.ID)
	if err != nil || observed.State != runner.Exited || observed.ProofID != attempts[0].Proof.ExitID {
		t.Fatalf("actual runner exit: %+v %v", observed, err)
	}
	if held, err := e.store.ExecutionDependencies(e.ctx); err != nil || len(held) != 0 {
		t.Fatalf("released execution retained dependencies: %+v %v", held, err)
	}
	if e.source.programCalls.Load() != 1 {
		t.Fatal("acquiring the original binding created a second process")
	}
}

func TestDisableDuringRealMCPSetupCannotPublishOrActivate(t *testing.T) {
	e := newEnvironment(t, true)
	result := make(chan error, 1)
	go func() { _, err := e.host.Activate(e.ctx, e.ref); result <- err }()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	waiting := true
	for waiting {
		select {
		case <-e.ctx.Done():
			t.Fatal(context.Cause(e.ctx))
		case err := <-result:
			t.Fatalf("setup did not remain preparing: %v", err)
		case <-ticker.C:
			_, err := os.Stat(filepath.Join(e.directory, "prepared"))
			waiting = errors.Is(err, os.ErrNotExist)
			if err != nil && !waiting {
				t.Fatal(err)
			}
		}
	}
	if err := e.host.SetEnabled(e.ctx, "echo", false); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("withdrawn preparation published")
		}
	case <-e.ctx.Done():
		t.Fatal(context.Cause(e.ctx))
	}
	if err := e.host.WaitRetired(e.ctx, "echo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.directory, "activated")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("withdrawn setup activated: %v", err)
	}
	attempts, err := e.store.Attempts(e.ctx)
	if err != nil || len(attempts) != 1 || !attempts[0].Released {
		t.Fatalf("late preparation lost protection: %+v %v", attempts, err)
	}
}
