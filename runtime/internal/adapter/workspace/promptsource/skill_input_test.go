package promptsource

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	sdk "github.com/Tangerg/scope/skills"
)

func TestSkillDocumentAdmissionPreservesKindAndSizeFailures(t *testing.T) {
	for _, test := range []struct {
		name     string
		prepare  func(string) error
		category error
		cause    error
	}{
		{
			name: "directory",
			prepare: func(path string) error {
				if err := os.Remove(path); err != nil {
					return err
				}
				return os.Mkdir(path, 0o755)
			},
			category: sdk.ErrInvalidSkill,
			cause:    fileinput.ErrNotRegular,
		},
		{
			name: "oversized",
			prepare: func(path string) error {
				return os.Truncate(path, domainskills.MaxAuthoredSkillDocumentBytes+1)
			},
			category: domainskills.ErrDocumentTooLarge,
			cause:    fileinput.ErrTooLarge,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeRuntimeSkill(t, root, "review", "Review the result.")
			if err := test.prepare(filepath.Join(root, "review", sdk.SkillFile)); err != nil {
				t.Fatal(err)
			}
			source, err := newRuntimeSkillSource(root, root)
			if err != nil {
				t.Fatal(err)
			}
			_, err = source.Load(t.Context(), "review")
			if !errors.Is(err, test.category) || !errors.Is(err, test.cause) {
				t.Fatalf("document admission lost its category or cause: %v", err)
			}
		})
	}
}
