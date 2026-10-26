package main

import (
	"path/filepath"
	"testing"
)

func TestWebApplicationRejectsMissingOrInvalidDistribution(t *testing.T) {
	directory := t.TempDir()
	for _, path := range []string{"relative", directory, filepath.Join(directory, "missing")} {
		_, err := newWebAssets(path)
		if err == nil {
			t.Fatalf("accepted invalid web distribution %q", path)
		}
	}
}
