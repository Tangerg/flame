package promptsource

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

func TestDiscoveryAndDetailShareExecutionSource(t *testing.T) {
	workspace, user := t.TempDir(), t.TempDir()
	project := ProjectSkillDir(workspace)
	writeRuntimeSkill(t, user, "shared", "user instructions")
	writeRuntimeSkill(t, project, "shared", "project instructions")
	catalog := NewSkills(user)
	detail, err := catalog.Get(t.Context(), workspace, "shared")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(project, "shared", sdk.SkillFile))
	if err != nil {
		t.Fatal(err)
	}
	physicalPath, err := filepath.EvalSymlinks(filepath.Join(project, "shared", sdk.SkillFile))
	if err != nil {
		t.Fatal(err)
	}
	if detail.Scope != domainskills.ScopeProject || detail.Path != physicalPath || detail.Revision != fmt.Sprintf("%x", sha256.Sum256(content)) {
		t.Fatalf("detail source = %+v", detail)
	}
	source, err := OverlaySkillSource(workspace, user, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := source.Load(t.Context(), "shared")
	if err != nil || loaded.Instructions != detail.Instructions {
		t.Fatalf("model and detail differ: %+v, %v", loaded, err)
	}
	writeRuntimeSkill(t, project, "shared", "edited instructions")
	edited, err := catalog.Get(t.Context(), workspace, "shared")
	if err != nil || edited.Revision == detail.Revision || edited.Instructions != "edited instructions" {
		t.Fatalf("detail retained stale content: %+v, %v", edited, err)
	}
}

func TestPartialSkillDiscoveryDoesNotExposeShadowedUserBundle(t *testing.T) {
	workspace, user := t.TempDir(), t.TempDir()
	project := ProjectSkillDir(workspace)
	writeRuntimeSkill(t, user, "broken", "must not fall through")
	writeRuntimeSkill(t, user, "working", "still discoverable")
	writeRuntimeSkill(t, project, "broken", "invalid content")
	if err := os.WriteFile(filepath.Join(project, "broken", sdk.SkillFile), []byte("malformed document"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := ListSkills(t.Context(), workspace, user)
	if err != nil || len(found.Skills) != 1 || found.Skills[0].Name != "working" || len(found.Diagnostics) != 1 || found.Diagnostics[0].Name != "broken" {
		t.Fatalf("partial discovery = %+v, %v", found, err)
	}
	source, err := OverlaySkillSource(workspace, user, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := source.List(t.Context())
	if err != nil || len(listed) != 1 || listed[0].Name != "working" {
		t.Fatalf("model discovery = %+v, %v", listed, err)
	}
	if _, err := source.Load(t.Context(), "broken"); !errors.Is(err, sdk.ErrInvalidSkill) {
		t.Fatalf("model loaded broken or shadowed bundle: %v", err)
	}
	if _, err := NewSkills(user).Get(t.Context(), workspace, "broken"); !errors.Is(err, workspaceapp.ErrSkillUnavailable) {
		t.Fatalf("detail loaded broken or shadowed bundle: %v", err)
	}
}

func writeRuntimeSkill(t *testing.T, root, name, body string) {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create skill %q: %v", name, err)
	}
	document := fmt.Sprintf("---\nname: %s\ndescription: A valid Runtime skill used by the bounded-source counterexample.\n---\n%s", name, body)
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatalf("write skill %q: %v", name, err)
	}
}

func TestRuntimeSkillSourceRejectsOversizedDocument(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "oversized", strings.Repeat("x", domainskills.MaxAuthoredSkillDocumentBytes))

	source, err := OverlaySkillSource("", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if source == nil {
		t.Fatal("configured source is nil")
	}
	if summary, err := source.Lookup(t.Context(), "oversized"); err != nil || summary.Name != "oversized" {
		t.Fatalf("Lookup must read metadata without loading the oversized instructions: %+v, %v", summary, err)
	}
	if _, err := source.Load(t.Context(), "oversized"); !errors.Is(err, domainskills.ErrDocumentTooLarge) {
		t.Fatalf("Load error = %v, want ErrDocumentTooLarge before the document is materialized", err)
	}
}

func TestMergedRuntimeSkillSourcePreservesBundleOwnership(t *testing.T) {
	workspace, userRoot := t.TempDir(), t.TempDir()
	projectRoot := ProjectSkillDir(workspace)
	writeRuntimeSkill(t, projectRoot, "project", "project instructions")
	writeRuntimeSkill(t, userRoot, "shared", "user instructions")
	source, err := OverlaySkillSource(workspace, userRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if skill, err := source.Load(t.Context(), "shared"); err != nil || skill.Instructions != "user instructions" {
		t.Fatalf("absent project bundle must resolve to user source: %+v, %v", skill, err)
	}
	if err := os.Mkdir(filepath.Join(projectRoot, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Load(t.Context(), "shared"); !errors.Is(err, sdk.ErrInvalidSkill) || errors.Is(err, sdk.ErrSkillNotFound) {
		t.Fatalf("broken project bundle must not expose the lower-precedence user copy: %v", err)
	}
}

func TestRuntimeSkillSourceRejectsDocumentGrowthAfterOpen(t *testing.T) {
	root := t.TempDir()
	name := "growing"
	document := "---\nname: growing\ndescription: A valid Runtime skill used by the bounded-source counterexample.\n---\n"
	document += strings.Repeat("x", domainskills.MaxAuthoredSkillDocumentBytes-len(document))
	path := filepath.Join(root, name, sdk.SkillFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}

	source, err := newRuntimeSkillSource(root, root)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.openSkillDocument(name)
	if err != nil {
		t.Fatalf("open document at exact limit: %v", err)
	}
	if appendFile, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		_ = file.close()
		t.Fatal(err)
	} else if _, err := appendFile.WriteString("x"); err != nil {
		_ = appendFile.Close()
		_ = file.close()
		t.Fatal(err)
	} else if err := appendFile.Close(); err != nil {
		_ = file.close()
		t.Fatal(err)
	}
	if _, err := readSkillDocument(t.Context(), name, file); !errors.Is(err, domainskills.ErrDocumentTooLarge) {
		t.Fatalf("grown document error = %v, want ErrDocumentTooLarge", err)
	}
}

func TestRuntimeSkillSourceRejectsOverCapacityDirectory(t *testing.T) {
	workspace := t.TempDir()
	root := ProjectSkillDir(workspace)
	for index := range domainskills.MaxSkillsPerSource + 1 {
		writeRuntimeSkill(t, root, fmt.Sprintf("skill-%03d", index), "instructions")
	}

	if _, err := ListSkills(t.Context(), workspace, ""); !errors.Is(err, domainskills.ErrLibraryCapacity) {
		t.Fatalf("ListSkills error = %v, want ErrLibraryCapacity beyond %d entries", err, domainskills.MaxSkillsPerSource)
	}
}

func TestRuntimeSkillSourceCapacityCountsOnlyValidSkills(t *testing.T) {
	workspace := t.TempDir()
	root := ProjectSkillDir(workspace)
	for index := range domainskills.MaxSkillsPerSource + 1 {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("invalid-%03d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := ListSkills(t.Context(), workspace, "")
	if err != nil {
		t.Fatalf("ListSkills rejected invalid candidates below the raw entry limit: %v", err)
	}
	if len(listed.Skills) != 0 {
		t.Fatalf("ListSkills = %+v, want no valid Skills", listed)
	}
}

func TestRuntimeSkillSourceRejectsRawDirectoryFlood(t *testing.T) {
	workspace := t.TempDir()
	root := ProjectSkillDir(workspace)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := range domainskills.MaxSkillDirectoryEntries + 1 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("junk-%03d", index)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ListSkills(t.Context(), workspace, ""); !errors.Is(err, domainskills.ErrLibraryCapacity) {
		t.Fatalf("ListSkills raw-directory error = %v, want ErrLibraryCapacity", err)
	}
}

func TestRuntimeSkillSourcesRejectEscapingProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	escaped := t.TempDir()
	writeRuntimeSkill(t, escaped, "escaped", "must not load")
	if err := os.MkdirAll(filepath.Join(workspace, ".flame"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escaped, ProjectSkillDir(workspace)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := ListSkills(t.Context(), workspace, ""); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("ListSkills error = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := OverlaySkillSource(workspace, "", nil); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("OverlaySkillSource error = %v, want ErrPathOutsideRoot", err)
	}
}

func TestRuntimeSkillSourcesAllowInWorkspaceAlias(t *testing.T) {
	workspace := t.TempDir()
	physical := filepath.Join(workspace, "skill-library")
	writeRuntimeSkill(t, physical, "inside", "allowed")
	if err := os.MkdirAll(filepath.Join(workspace, ".flame"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, ProjectSkillDir(workspace)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	listed, err := ListSkills(t.Context(), workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Skills) != 1 || listed.Skills[0].Name != "inside" || listed.Skills[0].Scope != domainskills.ScopeProject {
		t.Fatalf("ListSkills = %+v, want the confined project Skill", listed)
	}
}

func TestRuntimeSkillSourcesRejectBrokenExistingRoots(t *testing.T) {
	for _, test := range []struct {
		name  string
		build func(t *testing.T) (workspaceRoot, userDir string)
	}{
		{
			name: "project source is a file",
			build: func(t *testing.T) (string, string) {
				workspace := t.TempDir()
				if err := os.MkdirAll(filepath.Join(workspace, ".flame"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(ProjectSkillDir(workspace), []byte("not a directory"), 0o644); err != nil {
					t.Fatal(err)
				}
				return workspace, ""
			},
		},
		{
			name: "user source is a broken alias",
			build: func(t *testing.T) (string, string) {
				root := t.TempDir()
				alias := filepath.Join(root, "skills")
				if err := os.Symlink(filepath.Join(root, "missing"), alias); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return "", alias
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace, user := test.build(t)
			if _, err := ListSkills(t.Context(), workspace, user); err == nil {
				t.Fatal("ListSkills silently treated a broken source as absent")
			}
			if _, err := OverlaySkillSource(workspace, user, nil); err == nil {
				t.Fatal("OverlaySkillSource silently treated a broken source as absent")
			}
		})
	}
}

func TestRuntimeSkillSourceRejectsOversizedResource(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "with-resource", "Read references/large.txt")
	resourceDir := filepath.Join(root, "with-resource", "references")
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(resourceDir, "large.txt"),
		[]byte(strings.Repeat("x", domainskills.MaxSkillResourceBytes+1)),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	source, err := OverlaySkillSource("", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sdk.ReadResource(t.Context(), source, "with-resource", "references/large.txt", domainskills.MaxSkillResourceBytes); !errors.Is(err, domainskills.ErrResourceTooLarge) {
		t.Fatalf("ReadResource error = %v, want ErrResourceTooLarge", err)
	}
}

func TestRuntimeSkillSourceRejectsResourceGrowthAfterOpen(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "growing-resource", "Read references/growing.txt")
	resourceDir := filepath.Join(root, "growing-resource", "references")
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resourceDir, "growing.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", domainskills.MaxSkillResourceBytes)), 0o644); err != nil {
		t.Fatal(err)
	}

	source, err := OverlaySkillSource("", root, nil)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.OpenResource(t.Context(), "growing-resource", "references/growing.txt")
	if err != nil {
		t.Fatalf("OpenResource at exact limit: %v", err)
	}
	if appendFile, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	} else if _, err := appendFile.WriteString("x"); err != nil {
		_ = appendFile.Close()
		_ = file.Close()
		t.Fatal(err)
	} else if err := appendFile.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if !errors.Is(errors.Join(readErr, closeErr), domainskills.ErrResourceTooLarge) {
		t.Fatalf("grown resource error = %v, want ErrResourceTooLarge", errors.Join(readErr, closeErr))
	}
}
