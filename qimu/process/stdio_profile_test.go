package process_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/process"
)

type textLines struct{}

func (textLines) Name() qimu.Profile { return "test-lines" }

func (textLines) Bind(setup *qimu.Setup, _ process.Program) (process.Connect[io.ReadWriteCloser], error) {
	export := setup.Unit().Exports[0]
	return func(_ context.Context, stream io.ReadWriteCloser) (io.ReadWriteCloser, []qimu.ExportValue, error) {
		if _, err := io.WriteString(stream, "read\n"); err != nil {
			return stream, nil, err
		}
		line, err := bufio.NewReader(stream).ReadString('\n')
		if err != nil {
			return stream, nil, err
		}
		return stream, []qimu.ExportValue{{Name: export.Name, Contract: export.Contract, Value: line, Durable: true}}, nil
	}, nil
}

func TestLineProtocolHelper(t *testing.T) {
	if os.Getenv("QIMU_TEST_LINES") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	if line, err := reader.ReadString('\n'); err != nil || line != "read\n" {
		os.Exit(2)
	}
	if _, err := fmt.Fprintln(os.Stdout, "plain stdio"); err != nil {
		os.Exit(3)
	}
	_, _ = io.Copy(io.Discard, reader)
	os.Exit(0)
}

func TestNonMCPProcessPublishesAndRetiresUsingItsOwnProtocol(t *testing.T) {
	e := newEnvironment(t, false)
	program := e.source.program
	program.Args = []string{"-test.run=^TestLineProtocolHelper$"}
	program.Env = append(os.Environ(), "QIMU_TEST_LINES=1")
	driver, err := process.New(process.Config{Runner: e.runner, Store: e.store, Source: &source{program: program}, CloseTimeout: time.Second}, textLines{})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.host.RegisterDriver(driver); err != nil {
		t.Fatal(err)
	}
	descriptor := []byte(`{"wire":"utf8-lines"}`)
	ref := qimu.ContractRef{Name: "example.lines", Version: "1", Kind: qimu.ServiceKind, Mode: qimu.PortableMode, Digest: integrity.Hash(descriptor)}
	contract, err := qimu.BindContract[string](ref, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.host.Register(contract); err != nil {
		t.Fatal(err)
	}
	plugin, err := qimu.Define(qimu.Definition{Revision: integrity.Hash([]byte("lines release")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "worker", Profile: driver.Profile(), Exports: []qimu.ExportDeclaration{{Name: "answer", Contract: ref}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.host.Install(e.ctx, qimu.InstallationConfig{ID: "lines", Plugin: plugin, Configuration: []byte(`{}`), Enabled: true}); err != nil {
		t.Fatal(err)
	}
	target := qimu.ExportRef{InstallationID: "lines", Release: plugin.Revision(), UnitID: "worker", Export: "answer", Contract: ref}
	binding, err := e.host.Activate(e.ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	owner := qimu.NewLifetime(e.ctx)
	scope, err := e.host.Admission().OpenScope(e.ctx, "text")
	if err != nil {
		t.Fatal(err)
	}
	use, err := e.host.AcquireBinding(e.ctx, binding, qimu.UseConfig{Owner: owner, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if err = qimu.BorrowExport[string](e.ctx, use, target, func(_ context.Context, line string) error {
		if line != "plain stdio\n" {
			return fmt.Errorf("unexpected response %q", line)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = use.Close(e.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Close(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.host.SetEnabled(e.ctx, "lines", false); err != nil {
		t.Fatal(err)
	}
	if err = e.host.WaitRetired(e.ctx, "lines"); err != nil {
		t.Fatal(err)
	}
	attempts, err := e.store.Attempts(e.ctx)
	if err != nil || len(attempts) != 1 || !attempts[0].Released || attempts[0].Proof == nil || attempts[0].Attempt.Profile != driver.Profile() {
		t.Fatalf("original exit receipt: %+v, %v", attempts, err)
	}
}
