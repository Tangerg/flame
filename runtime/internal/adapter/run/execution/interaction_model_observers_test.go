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
	frozen := contributionBindingsWire(bindings)
	slices.Reverse(bindings)
	restored, err := decodeContributionBindings(frozen, contract)
	if err != nil || restored[0].ContributionID != "outer" || restored[1].ContributionID != "inner" || restored[0].ServerID != id {
		t.Fatalf("restore changed captured chain: %+v, %v", restored, err)
	}
	session := &interactionSession{modelObservers: restored, compositionActivations: []plugin.Activation{{Identity: testsupport.Digest("activation"), Dependencies: []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("observer release")}}}}}
	if actual := session.installationDependencies(); !reflect.DeepEqual(actual, []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("observer release")}}) {
		t.Fatalf("observer release is not held: %+v", actual)
	}
	frozen[1] = frozen[0]
	if _, err := decodeContributionBindings(frozen, contract); err == nil {
		t.Fatal("duplicate retained observer was accepted")
	}
}

func TestCapturedModelContextRetainsExactContractOrderAndRelease(t *testing.T) {
	observerContract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	server := testsupport.ServerName("context")
	origin, err := mcpserver.InstallationOrigin(dependedInstallation)
	if err != nil {
		t.Fatal(err)
	}
	id, err := mcpserver.NewID(origin, server)
	if err != nil {
		t.Fatal(err)
	}
	bindings := []plugins.ContributionBinding{{InstallationID: dependedInstallation, Digest: testsupport.Digest("context release"), ContributionID: "first", ServerID: id, ServerName: server, Export: "first", Contract: contextContract.Identity()}, {InstallationID: dependedInstallation, Digest: testsupport.Digest("context release"), ContributionID: "second", ServerID: id, ServerName: server, Export: "second", Contract: contextContract.Identity()}}
	wire := contributionBindingsWire(bindings)
	restored, err := decodeContributionBindings(wire, contextContract)
	if err != nil || !reflect.DeepEqual(restored, bindings) {
		t.Fatalf("captured context binding changed: %+v %v", restored, err)
	}
	if _, err := decodeContributionBindings(wire, observerContract); err == nil {
		t.Fatal("context was reinterpreted as observer")
	}
	session := &interactionSession{modelContexts: restored, modelObservers: restored[:1], compositionActivations: []plugin.Activation{{Identity: testsupport.Digest("activation"), Dependencies: []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("context release")}}}}}
	want := []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("context release")}}
	if actual := session.installationDependencies(); !reflect.DeepEqual(actual, want) {
		t.Fatalf("context release not held once: %+v", actual)
	}
	wire[1] = wire[0]
	if _, err := decodeContributionBindings(wire, contextContract); err == nil {
		t.Fatal("duplicate context binding accepted")
	}
}
