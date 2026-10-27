package promptsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/scope/skills"
)

func TestExecutionSkillSourcePreservesCancellationClassAndCause(t *testing.T) {
	for _, scope := range []string{"project", "user", "package", "empty"} {
		t.Run(scope, func(t *testing.T) {
			workspace, user := t.TempDir(), t.TempDir()
			var packages PackageSkills
			switch scope {
			case "project":
				writeRuntimeSkill(t, ProjectSkillDir(workspace), "review", "Review the result.")
			case "user":
				writeRuntimeSkill(t, user, "review", "Review the result.")
			case "package":
				bundle := PackageSkillBundle{Root: t.TempDir(), Skills: []plugin.Skill{{Name: "review", Description: runtimeSkillDescription}}, Dependency: plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("release")}}
				writeRuntimeSkill(t, bundle.Root, "review", "Review the result.")
				packages = packageSkillFixtures{bundle}
			}
			source, _, err := OverlaySkillSource(t.Context(), workspace, user, packages, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("execution source retired")
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(cause)
			for _, operation := range []struct {
				name string
				call func() error
			}{
				{"list", func() error { _, err := source.List(ctx); return err }},
				{"lookup", func() error { _, err := source.Lookup(ctx, "review"); return err }},
				{"load", func() error { _, err := source.Load(ctx, "review"); return err }},
				{"resource", func() error {
					file, err := source.OpenResource(ctx, "review", sdk.SkillFile)
					if file != nil {
						err = errors.Join(err, file.Close())
					}
					return err
				}},
			} {
				t.Run(operation.name, func(t *testing.T) {
					err := operation.call()
					if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
						t.Fatalf("source lost cancellation class or cause: %v", err)
					}
				})
			}
		})
	}
}

type cancelingResourceFile struct {
	*os.File
	cancel  func()
	failure error
}

func (f cancelingResourceFile) Read(buffer []byte) (int, error) {
	n, err := f.File.Read(buffer)
	f.cancel()
	return n, errors.Join(err, f.failure)
}

type retiringPackageRead struct {
	PackageSkills
	cancel  func()
	failure error
}

func (p retiringPackageRead) ReadSkillResource(ctx context.Context, dependency plugin.Dependency, name, resource string) ([]byte, error) {
	content, err := p.PackageSkills.ReadSkillResource(ctx, dependency, name, resource)
	p.cancel()
	return content, errors.Join(err, p.failure)
}

func TestPackageSkillReadsPreserveConcurrentCancellationAndFailure(t *testing.T) {
	for _, operation := range []string{"load", "resource"} {
		t.Run(operation, func(t *testing.T) {
			bundle := PackageSkillBundle{Root: t.TempDir(), Skills: []plugin.Skill{{Name: "review", Description: runtimeSkillDescription}}, Dependency: plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("release")}}
			writeRuntimeSkill(t, bundle.Root, "review", "Review the result.")
			cause, failure := errors.New("package read retired"), errors.New("package I/O failed")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			source := packageSkillSource{
				bundle: bundle,
				authority: retiringPackageRead{
					PackageSkills: packageSkillFixtures{bundle}, cancel: func() { cancel(cause) }, failure: failure,
				},
			}
			var err error
			if operation == "load" {
				_, err = source.Load(ctx, "review")
			} else {
				file, openErr := source.OpenResource(ctx, "review", sdk.SkillFile)
				err = openErr
				if file != nil {
					t.Error("canceled package resource returned a usable file")
					err = errors.Join(err, file.Close())
				}
			}
			for _, expected := range []error{context.Canceled, cause, failure} {
				if !errors.Is(err, expected) {
					t.Errorf("package read lost failure %v: %v", expected, err)
				}
			}
		})
	}
}

func TestSkillResourceReadKeepsBoundAndConcurrentFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", domainskills.MaxSkillResourceBytes)), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.Truncate(path, domainskills.MaxSkillResourceBytes+1); err != nil {
		t.Fatal(err)
	}
	cause, failure := errors.New("resource read retired"), errors.New("resource I/O failed")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	resource := boundedSkillResource{
		File: cancelingResourceFile{File: file, cancel: func() { cancel(cause) }, failure: failure},
		ctx:  ctx, name: "review", resource: "reference.txt",
	}
	n, err := resource.Read(make([]byte, domainskills.MaxSkillResourceBytes+1))
	if n > domainskills.MaxSkillResourceBytes {
		t.Errorf("cancellation exposed %d bytes beyond the resource limit", n-domainskills.MaxSkillResourceBytes)
	}
	for _, expected := range []error{context.Canceled, cause, failure, domainskills.ErrResourceTooLarge} {
		if !errors.Is(err, expected) {
			t.Errorf("resource read lost failure %v: %v", expected, err)
		}
	}
}
