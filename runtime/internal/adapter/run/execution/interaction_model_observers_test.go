package execution

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

func TestCapturedModelObserversRetainIdentityOrderAndInstallationDependencies(t *testing.T) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	server := testsupport.ServerName("observer")
	origin, err := mcpserver.InstallationOrigin(dependedInstallation)
	if err != nil {
		t.Fatal(err)
	}
	id, err := mcpserver.NewID(origin, server)
	if err != nil {
		t.Fatal(err)
	}
	bindings := []plugins.ContributionBinding{
		{InstallationID: dependedInstallation, Digest: testsupport.Digest("observer release"), ContributionID: "outer", ServerID: id, ServerName: server, Export: "outer", Contract: contract.Identity()},
		{InstallationID: dependedInstallation, Digest: testsupport.Digest("observer release"), ContributionID: "inner", ServerID: id, ServerName: server, Export: "inner", Contract: contract.Identity()},
	}
	frozen := observerBindingsWire(bindings)
	slices.Reverse(bindings)
	restored, err := decodeObserverBindings(frozen)
	if err != nil || restored[0].ContributionID != "outer" || restored[1].ContributionID != "inner" || restored[0].ServerID != id {
		t.Fatalf("restore changed captured chain: %+v, %v", restored, err)
	}
	session := &interactionSession{modelObservers: restored}
	if actual := session.installationDependencies(); !reflect.DeepEqual(actual, []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("observer release")}}) {
		t.Fatalf("observer release is not held: %+v", actual)
	}
	frozen[1] = frozen[0]
	if _, err := decodeObserverBindings(frozen); err == nil {
		t.Fatal("duplicate retained observer was accepted")
	}
}
