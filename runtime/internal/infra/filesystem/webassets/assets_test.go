package webassets_test

import (
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/webassets"
	"path/filepath"
	"testing"
)

func TestWebApplicationRejectsMissingOrInvalidDistribution(t *testing.T) {
	directory := t.TempDir()
	for _, path := range []string{"relative", directory, filepath.Join(directory, "missing")} {
		_, err := webassets.New(path)
		if err == nil {
			t.Fatalf("accepted invalid web distribution %q", path)
		}
	}
}
