package bootstrap

import (
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
	modernsqlite "modernc.org/sqlite"
)

const (
	pluginEnablementCrashHome   = "FLAME_TEST_PLUGIN_ENABLEMENT_HOME"
	pluginEnablementCrashRunner = "FLAME_TEST_PLUGIN_ENABLEMENT_RUNNER"
	pluginEnablementCrashKey    = "disable-original-plugin-installation"
	pluginEnablementFixture     = "enablement-installation"
)

func TestPluginEnablementAcknowledgementCrashChild(t *testing.T) {
	home := os.Getenv(pluginEnablementCrashHome)
	if home == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_exit_before_enablement_acknowledgement", 0,
		func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, os.Getenv(pluginEnablementCrashRunner)))
	installed := installExternalConnector(r)
	if err := os.WriteFile(filepath.Join(home, pluginEnablementFixture), []byte(installed.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, `CREATE TRIGGER interrupt_plugin_enablement_acknowledgement
		BEFORE UPDATE OF payload ON idempotency_records
		WHEN OLD.key = '`+pluginEnablementCrashKey+`' AND NEW.kind = 'response'
		BEGIN SELECT test_exit_before_enablement_acknowledgement(); END`)
	request := protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}
	result := r.endpoint.Invoke(t.Context(), delivery.PluginsSetEnablement, request, protocol.CallMetadata{IdempotencyKey: pluginEnablementCrashKey})
	t.Fatalf("enablement did not stop before acknowledging: %+v", result)
}

func TestPluginEnablementCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	home, runnerDir := testsupport.TempDirectory(t), testsupport.TempDirectory(t)
	if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPluginEnablementAcknowledgementCrashChild$", "-test.timeout=35s")
	child.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), pluginEnablementCrashHome+"="+home, pluginEnablementCrashRunner+"="+runnerDir)
	child.WaitDelay = 3 * time.Second
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("enablement child did not exit while writing the receipt: %v\n%s", err, output)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER interrupt_plugin_enablement_acknowledgement")
	savedID, err := os.ReadFile(filepath.Join(home, pluginEnablementFixture))
	if err != nil {
		t.Fatal(err)
	}
	id := string(savedID)
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, runnerDir))
	requireEnabled := func(want bool) {
		t.Helper()
		if installation := r.pluginInstallation(id); installation.Enabled != want {
			t.Fatalf("installation enabled = %t, want %t", installation.Enabled, want)
		}
	}
	requireEnabled(true)
	disable := protocol.SetPluginEnablementRequest{InstallationID: id, Enabled: false}
	first := r.must(delivery.PluginsSetEnablement, disable, pluginEnablementCrashKey).(*protocol.PluginEnablement)
	requireEnabled(false)
	enable := protocol.SetPluginEnablementRequest{InstallationID: id, Enabled: true}
	r.must(delivery.PluginsSetEnablement, enable, "reenable-original-plugin-installation")
	replayed := r.must(delivery.PluginsSetEnablement, disable, pluginEnablementCrashKey).(*protocol.PluginEnablement)
	if *first != (protocol.PluginEnablement{InstallationID: id}) || *replayed != *first {
		t.Fatalf("original enablement receipts differ: %+v, %+v", first, replayed)
	}
	requireEnabled(true)

	const confirmedEnabled = "confirm-already-enabled-installation"
	unchanged := r.must(delivery.PluginsSetEnablement, enable, confirmedEnabled).(*protocol.PluginEnablement)
	r.must(delivery.PluginsSetEnablement, disable, "disable-again-after-confirmation")
	replayed = r.must(delivery.PluginsSetEnablement, enable, confirmedEnabled).(*protocol.PluginEnablement)
	if *unchanged != (protocol.PluginEnablement{InstallationID: id, Enabled: true}) || *replayed != *unchanged {
		t.Fatalf("unchanged enablement receipts differ: %+v, %+v", unchanged, replayed)
	}
	requireEnabled(false)
}

func TestPluginEnablementReturnsTheDecisionAfterFrameConfirmationFails(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	id := installExternalConnector(r).ID
	questionCrashDatabase(t, cfg.stores.DataDirectory, `CREATE TRIGGER refuse_plugin_enablement_confirmation
		BEFORE UPDATE OF state ON frame_submissions
		WHEN OLD.state = 'committed' AND NEW.state = 'committed_synchronized'
		BEGIN SELECT RAISE(ABORT, 'plugin enablement confirmation refused'); END`)

	const key = "disable-with-refused-confirmation"
	request := protocol.SetPluginEnablementRequest{InstallationID: id}
	result := r.must(delivery.PluginsSetEnablement, request, key).(*protocol.PluginEnablement)
	if *result != (protocol.PluginEnablement{InstallationID: id}) {
		t.Fatalf("enablement despite refused confirmation = %+v", result)
	}
	questionCrashDatabase(t, cfg.stores.DataDirectory, "DROP TRIGGER refuse_plugin_enablement_confirmation")
	r = reopenPluginRuntime(t, r, &cfg)
	replayed := r.must(delivery.PluginsSetEnablement, request, key).(*protocol.PluginEnablement)
	if *replayed != *result || r.pluginInstallation(id).Enabled {
		t.Fatalf("recovered enablement = %+v, original %+v", replayed, result)
	}
}
