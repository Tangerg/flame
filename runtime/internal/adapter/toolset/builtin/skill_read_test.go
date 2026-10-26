package builtin

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	toolcontract "github.com/Tangerg/scope/core/tool"
	skillspec "github.com/Tangerg/scope/skills"
	skillstool "github.com/Tangerg/scope/tools/skills"
)

type recordingProbe struct{ names []string }

func (r *recordingProbe) RecordUse(_ context.Context, name string, _ time.Time) error {
	r.names = append(r.names, name)
	return nil
}

type stubResourceSource struct{ loadErr error }

func (stubResourceSource) List(context.Context) ([]skillspec.Summary, error) { return nil, nil }

func (stubResourceSource) Lookup(_ context.Context, name string) (skillspec.Summary, error) {
	return skillspec.Summary{Name: name, Description: "d"}, nil
}

func (s stubResourceSource) Load(_ context.Context, name string) (*skillspec.Skill, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return &skillspec.Skill{Frontmatter: skillspec.Frontmatter{Name: name, Description: "d"}, Instructions: "b"}, nil
}

func (stubResourceSource) OpenResource(context.Context, string, string) (fs.File, error) {
	return nil, errors.New("not implemented")
}

func TestRecordingSourceRecordsOnSuccessfulLoad(t *testing.T) {
	probe := &recordingProbe{}
	src := recordingSource{ResourceSource: stubResourceSource{}, recorder: probe}
	if _, err := src.Load(context.Background(), "run-tests"); err != nil {
		t.Fatal(err)
	}
	if len(probe.names) != 1 || probe.names[0] != "run-tests" {
		t.Fatalf("recorded = %v, want [run-tests]", probe.names)
	}
}

func TestRecordingSourceSkipsUseOnLoadError(t *testing.T) {
	probe := &recordingProbe{}
	src := recordingSource{ResourceSource: stubResourceSource{loadErr: errors.New("absent")}, recorder: probe}
	if _, err := src.Load(context.Background(), "ghost"); err == nil {
		t.Fatal("expected a load error")
	}
	if len(probe.names) != 0 {
		t.Fatalf("recorded a use for a failed load: %v", probe.names)
	}
}

func TestSkillLoadReportsUsageFailureAndRecovers(t *testing.T) {
	var diagnostics bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	userDir := t.TempDir()
	const name = "run-tests"
	writeSkill(t, userDir, name, "Run the project tests before completing a change.")
	usagePath := filepath.Join(userDir, ".usage.json")
	if err := os.Mkdir(usagePath, 0o755); err != nil {
		t.Fatal(err)
	}
	recorder, err := skillauthoring.NewStore(userDir, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	storageErr := recorder.RecordUse(t.Context(), name, time.Now())
	if storageErr == nil {
		t.Fatal("invalid usage file did not fail recording")
	}
	tools, _, err := BuildReaders(t.Context(), t.TempDir(), userDir, recorder, nil)
	if err != nil {
		t.Fatal(err)
	}
	var loader toolcontract.Tool
	for _, candidate := range tools {
		if candidate.Definition().Name == "load_skill" {
			loader = candidate
		}
	}
	if loader == nil {
		t.Fatal("load_skill was not available")
	}
	loaded, err := callTextTool(t.Context(), loader, `{"name":"run-tests"}`)
	if err != nil || !strings.Contains(loaded, "instructions for "+name) {
		t.Fatalf("usage failure changed skill load = (%q, %v)", loaded, err)
	}
	if output := diagnostics.String(); !strings.Contains(output, name) || !strings.Contains(output, storageErr.Error()) {
		t.Errorf("usage failure lost its skill or cause: %s", output)
	}
	if err := os.Remove(usagePath); err != nil {
		t.Fatal(err)
	}
	diagnostics.Reset()
	if _, err := callTextTool(t.Context(), loader, `{"name":"run-tests"}`); err != nil {
		t.Fatalf("load after storage recovery: %v", err)
	}
	if info, err := os.Stat(usagePath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("usage recording did not recover: %v", err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("successful usage recording reported failure: %s", diagnostics.String())
	}
}

func writeSkill(t *testing.T, root, name, desc string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\ninstructions for " + name
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// TestBuild_MergesProjectOverUser proves the engine's skill tool
// layers <cwd>/.flame/skills over the user dir, with the project copy
// winning on a name collision.
func TestBuildReadersMergesProjectOverUser(t *testing.T) {
	cwd := t.TempDir()
	user := t.TempDir()

	writeSkill(t, promptsource.ProjectSkillDir(cwd), "shared", "PROJECT copy")
	writeSkill(t, promptsource.ProjectSkillDir(cwd), "proj-only", "project only")
	writeSkill(t, user, "shared", "USER copy")
	writeSkill(t, user, "user-only", "user only")

	tools, _, err := BuildReaders(t.Context(), cwd, user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("Build returned %d tools, want 3", len(tools))
	}
	byName := make(map[string]toolcontract.Tool, len(tools))
	for _, candidate := range tools {
		byName[candidate.Definition().Name] = candidate
	}

	list, err := callTextTool(context.Background(), byName["list_skills"], `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var page skillstool.ListPage
	if err := json.Unmarshal([]byte(list), &page, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	want := []skillspec.Summary{
		{Name: "proj-only", Description: "project only"},
		{Name: "shared", Description: "PROJECT copy"},
		{Name: "user-only", Description: "user only"},
	}
	if !slices.Equal(page.Skills, want) || page.NextCursor != nil {
		t.Fatalf("merged skill catalog = %+v", page)
	}

	loaded, err := callTextTool(context.Background(), byName["load_skill"], `{"name":"shared"}`)
	if err != nil {
		t.Fatalf("load shared: %v", err)
	}
	if !strings.Contains(loaded, "instructions for shared") {
		t.Errorf("load did not return the instruction body:\n%s", loaded)
	}
}

// TestBuild_AbsentWhenNoDirs proves the tool is omitted entirely when
// neither the project nor the user Skills directory exists — no empty Skill
// tool cluttering the model's tool list.
func TestBuildReadersAbsentWhenNoDirs(t *testing.T) {
	tools, _, err := BuildReaders(t.Context(), t.TempDir(), filepath.Join(t.TempDir(), "missing"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("Build returned %d tools without a skills directory", len(tools))
	}
}

func TestBuiltSkillReaderKeepsItsSelectedLibraryAfterDeletion(t *testing.T) {
	workspace, user := t.TempDir(), t.TempDir()
	project := promptsource.ProjectSkillDir(workspace)
	writeSkill(t, project, "shared", "project copy")
	writeSkill(t, user, "shared", "user copy")
	recorder := &recordingProbe{}
	tools, _, err := BuildReaders(t.Context(), workspace, user, recorder, nil)
	if err != nil {
		t.Fatal(err)
	}
	var loader toolcontract.Tool
	for _, candidate := range tools {
		if candidate.Definition().Name == "load_skill" {
			loader = candidate
		}
	}
	if loader == nil {
		t.Fatal("load_skill was not available")
	}
	if err := os.RemoveAll(filepath.Join(project, "shared")); err != nil {
		t.Fatal(err)
	}
	if loaded, err := callTextTool(t.Context(), loader, `{"name":"shared"}`); err == nil || loaded != "" {
		t.Fatalf("existing reader fell through to the user library: %q, %v", loaded, err)
	}
	if len(recorder.names) != 0 {
		t.Fatalf("a shadowed library acquired usage: %v", recorder.names)
	}
	tools, _, err = BuildReaders(t.Context(), workspace, user, recorder, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range tools {
		if candidate.Definition().Name == "load_skill" {
			loaded, err := callTextTool(t.Context(), candidate, `{"name":"shared"}`)
			if err != nil || !strings.Contains(loaded, "instructions for shared") || len(recorder.names) != 1 || recorder.names[0] != "shared" {
				t.Fatalf("fresh reader did not select and record the user library: %q, %v, %v", loaded, recorder.names, err)
			}
			return
		}
	}
	t.Fatal("fresh load_skill was not available")
}
