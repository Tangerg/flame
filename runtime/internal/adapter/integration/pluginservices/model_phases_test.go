package pluginservices

import (
	"errors"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestContributionRecursionUsesSourceIdentityAcrossActivationCapture(t *testing.T) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	installation := testsupport.InstallationID(t)
	release := testsupport.Digest("observer release")
	binding := plugins.ContributionBinding{
		InstallationID: installation,
		ContributionID: "observer",
		Digest:         release,
		ServerID:       testsupport.InstallationMCPServer(installation, "observer"),
		ServerName:     testsupport.ServerName("observer"),
		Export:         "observer",
		Contract:       contract.Identity(),
	}
	activation := testsupport.ServerActivation("configuration", "observer", plugin.Dependency{InstallationID: installation, Digest: release})
	captured := binding
	captured.Activation = &activation
	restored := binding
	copy := activation.Clone()
	restored.Activation = &copy
	for _, test := range []struct {
		name   string
		parent plugins.ContributionBinding
		child  plugins.ContributionBinding
	}{
		{"capture attaches original execution", binding, captured},
		{"restore copies the same execution", captured, restored},
		{"uncaptured request reenters captured execution", captured, binding},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent, err := enterContributionPhase(t.Context(), test.parent)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := enterContributionPhase(parent, test.child); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("activation projection bypassed recursive-contribution refusal: %v", err)
			}
		})
	}
	parent, err := enterContributionPhase(t.Context(), captured)
	if err != nil {
		t.Fatal(err)
	}
	different := restored
	different.ContributionID = "other-observer"
	child, err := enterContributionPhase(parent, different)
	if err != nil {
		t.Fatalf("a different contribution was rejected: %v", err)
	}
	if _, err := enterContributionPhase(child, binding); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("nested path lost its original ancestor: %v", err)
	}
	if _, err := enterContributionPhase(parent, different); err != nil {
		t.Fatalf("a sibling request mutated the parent's active path: %v", err)
	}
}
