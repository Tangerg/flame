package plugin_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestCommandAdmissionAndRestorationRequireTheOriginalInputContract(t *testing.T) {
	provider := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("release")}
	record := plugin.CommandBinding{
		ID: "original", Method: "records.update", Input: testsupport.JSONInput(t, `{}`),
		Contract: testsupport.CommandContract(t, "records.update", extension.ScopeInstallation),
		Provider: provider, Activation: testsupport.ServerActivation("activation", "backend", provider),
		Server: testsupport.ServerName("backend"), Contribution: "records", Export: "operations",
		Invocation: portable.Invocation{ID: "original", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: provider.InstallationID.String()}},
	}
	command, err := plugin.NewCommand(record)
	if err != nil {
		t.Fatal(err)
	}
	record = command.Snapshot()
	record.Input = testsupport.JSONInput(t, `false`)
	if _, err := plugin.NewCommand(record); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("new command admitted input outside its contract: %v", err)
	}
	if _, err := plugin.RestoreCommand(record); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("restoration admitted input outside its contract: %v", err)
	}
}
