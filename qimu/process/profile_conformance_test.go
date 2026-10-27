package process_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

func TestRealProcessDriverRejectsNativeObjectsBeforeCreationOrBusiness(t *testing.T) {
	for _, direction := range []string{"export", "import"} {
		t.Run(direction, func(t *testing.T) {
			e := newEnvironment(t, false)
			native, err := qimu.NewContract[func() *int]("document.shared-native", "1")
			if err != nil {
				t.Fatal(err)
			}
			if err := e.host.Register(native); err != nil {
				t.Fatal(err)
			}
			declaration := qimu.UnitDeclaration{ID: "backend", Profile: qimu.MCPProcess, Exports: []qimu.ExportDeclaration{{Name: "native", Contract: native.Ref()}}}
			if direction == "import" {
				declaration.Exports = []qimu.ExportDeclaration{{Name: "echo", Contract: e.ref.Contract}}
				declaration.Imports = []qimu.ImportDeclaration{{Slot: "shared", Contract: native.Ref(), Optional: true}}
			}
			plugin, err := qimu.Define(qimu.Definition{Revision: qimu.Hash([]byte(direction)), Units: []qimu.Unit{{Declaration: declaration}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.host.Install(e.ctx, qimu.InstallationConfig{ID: "invalid-native", Plugin: plugin, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			root := qimu.ExportRef{InstallationID: "invalid-native", Release: plugin.Revision(), UnitID: "backend", Export: declaration.Exports[0].Name, Contract: declaration.Exports[0].Contract}
			if _, err := e.host.Activate(e.ctx, root); !errors.Is(err, qimu.ErrContractMismatch) {
				t.Fatalf("process profile accepted a synchronous shared object: %v", err)
			}
			if e.source.programCalls.Load() != 0 {
				t.Fatal("incompatible profile reached executable selection")
			}
			if _, err := os.Stat(filepath.Join(e.directory, "prepared")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("incompatible profile reached the actual backend setup: %v", err)
			}
			// The same real runner, store and driver must still admit Portable
			// execution after rejecting the native contract.
			binding, err := e.host.Activate(e.ctx, e.ref)
			if err != nil {
				t.Fatal(err)
			}
			if len(binding.Roots) != 1 || binding.Roots[0] != e.ref || e.source.programCalls.Load() != 1 {
				t.Fatal("profile rejection changed the independent Portable binding")
			}
			if err := e.host.SetEnabled(context.WithoutCancel(e.ctx), "echo", false); err != nil {
				t.Fatal(err)
			}
			if err := e.host.WaitRetired(e.ctx, "echo"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
