package bootstrap

import (
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

func TestSkillDiscoveryRestoresOriginalReleaseAndRefusesForeignWorkspace(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "skills")
	first := poMCPActivate(r, source)
	waitCallContributorState(t, r, first.ID, protocol.MCPServerConnected)
	consumer := func() *pluginservices.SkillGovernance {
		files, err := skillauthoring.NewStore(cfg.skillsUserDir, skills.ScopeUser)
		if err != nil {
			t.Fatal(err)
		}
		publisher, err := workspaceadapter.NewSkillLibraries(files, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		calls := r.host.application.delivery.PluginCalls.(*pluginservices.CommandCalls)
		service, err := pluginservices.NewSkillGovernance(calls, publisher)
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	original := skilllearning.Candidates{Library: "original-project", Items: []skilllearning.Candidate{{Name: "verify", Scope: skill.User, Source: "user"}}}
	capture := &promptsource.SkillCapture{}
	result, err := consumer().Discover(t.Context(), original, capture)
	if err != nil || capture.Command == "" || len(result.Entries) != 1 {
		t.Fatalf("capture = %+v %+v %v", capture, result, err)
	}
	second := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-other").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: second.ID, Enabled: true}, "enable-other")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: skilllearning.Reference, InstallationID: second.ID, Contribution: "skill-learning"}, "select-other")
	waitCallContributorState(t, r, second.ID, protocol.MCPServerConnected)
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, first.ID, protocol.MCPServerConnected)
	withNewReader := original
	withNewReader.Items = append(withNewReader.Items, skilllearning.Candidate{Name: "verify", Scope: skill.Project, Source: "project"})
	restore := &promptsource.SkillCapture{Restore: true, Command: capture.Command}
	got, err := consumer().Discover(t.Context(), withNewReader, restore)
	if err != nil || !reflect.DeepEqual(got, result) || !restore.Activation.Equal(capture.Activation) {
		t.Fatalf("restoration rebound discovery: %+v %v", got, err)
	}
	foreign := withNewReader
	foreign.Library = "different-project"
	if _, err = consumer().Discover(t.Context(), foreign, restore); !errors.Is(err, plugin.ErrStale) {
		t.Fatalf("foreign workspace reused capture: %v", err)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "retire-original")
	if _, err = consumer().Discover(t.Context(), withNewReader, restore); err == nil {
		t.Fatal("replacement supplied an original capture")
	}
	fresh, err := consumer().Discover(t.Context(), withNewReader, nil)
	if err != nil || len(fresh.Entries) != 1 || fresh.Entries[0].Source != "project" {
		t.Fatalf("new discovery did not go to selected provider: %+v %v", fresh, err)
	}
}
