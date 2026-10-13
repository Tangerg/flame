package bootstrap

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestDiffComponentUsesCanonicalWorkspaceReadsAndWithdrawalPreservesFiles(t *testing.T) {
	cfg := poMCPConfig(t)
	root := cfg.DefaultWorkspacePath
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", root, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Diff Test", "-c", "user.email=diff@example.invalid"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--initial-branch=main")
	write("before.txt", "first\nsecond\n")
	git("add", ".")
	git("commit", "-m", "baseline")
	git("mv", "before.txt", "after.txt")
	write("added.txt", "<script> must remain text\nsecond\n")
	write("binary.dat", "\x00\x01\x02")
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs("../../../plugins/diff")
	if err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Scope != "workspace" || installed.Selected.Components[0].Material != "diff" {
		t.Fatalf("diff component admission: %+v", installed.Selected)
	}
	component := installed.Selected.Components[0]
	resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	read := protocol.GetDiffRequest{Workspace: protocol.WorkspaceRef{Path: root}, Format: protocol.DiffFormatRows}
	compare := func() {
		t.Helper()
		got := r.must(delivery.WorkspaceDiffGet, read, "").(*protocol.Diff)
		if got.Baseline.Type != protocol.DiffBaselineHead && got.Baseline.Type != protocol.DiffBaselineMergeBase {
			t.Fatalf("canonical comparison lost baseline evidence: %+v", got)
		}
	}
	for _, mode := range []protocol.DiffMode{protocol.DiffModeWorktree, protocol.DiffModeBase} {
		read.Mode = mode
		compare()
	}
	read.Mode = protocol.DiffModeWorktree
	diff := r.must(delivery.WorkspaceDiffGet, read, "").(*protocol.Diff)
	renamed, binary := false, false
	for _, file := range diff.Files {
		renamed = renamed || file.PreviousPath == "before.txt"
		binary = binary || file.Binary && file.Added == nil && file.Removed == nil
	}
	if !renamed || !binary {
		t.Fatalf("canonical rename or binary evidence missing: %+v", diff)
	}
	limit := 1
	read.Limit = &limit
	compare()
	if bounded := r.must(delivery.WorkspaceDiffGet, read, "").(*protocol.Diff); !bounded.Truncated {
		t.Fatalf("bounded diff did not report truncation: %+v", bounded)
	}
	read.Limit = nil
	authored, err := os.ReadFile(filepath.Join(source, component.Entry))
	if err != nil {
		t.Fatal(err)
	}
	projection := r.must(delivery.PluginsReadResource, resource, "").(*protocol.PluginResource)
	body, err := base64.StdEncoding.DecodeString(projection.Body)
	if err != nil || string(body) != string(authored) {
		t.Fatalf("component bytes changed: %v", err)
	}
	refuse := func(method delivery.Name, input any, want error) {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, input, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if !errors.Is(result.Failure, want) {
			t.Fatalf("%s refusal: %v, want %v", method, result.Failure, want)
		}
	}
	stale := resource
	stale.Digest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	refuse(delivery.PluginsReadResource, stale, protocol.ErrPluginStale)
	wrong := read
	wrong.Path = "../outside"
	refuse(delivery.WorkspaceDiffGet, wrong, protocol.ErrPathOutsideRoot)
	wrong = read
	wrong.Workspace.Path = t.TempDir()
	refuse(delivery.WorkspaceDiffGet, wrong, protocol.ErrVcsUnavailable)
	wrong = read
	wrong.Mode = "invalid"
	refuse(delivery.WorkspaceDiffGet, wrong, protocol.ErrInvalidParams)
	r.close()
	r = poMCPOpen(t, cfg)
	compare()
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-diff")
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginDisabled)
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall-diff")
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginNotFound)
	if got := r.must(delivery.WorkspaceDiffGet, read, "").(*protocol.Diff); !reflect.DeepEqual(got, diff) {
		t.Fatal("uninstall changed workspace evidence")
	}
	if content, err := os.ReadFile(filepath.Join(root, "added.txt")); err != nil || string(content) != "<script> must remain text\nsecond\n" {
		t.Fatalf("uninstall changed files: %v", err)
	}
}
