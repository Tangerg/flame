package backend

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
)

func TestRunRefusesARelativeDataDirectory(t *testing.T) {
	t.Setenv(process.DataVariable, "relative/data")
	served := false
	err := run(t.Context(), "example", func(context.Context, string) error {
		served = true
		return nil
	})
	if err == nil || served {
		t.Fatalf("run = %v, served %t; want refusal before serving", err, served)
	}
}

func TestRunServesTheLeasedDataDirectory(t *testing.T) {
	data := t.TempDir()
	t.Setenv(process.DataVariable, data)
	var got string
	if err := run(t.Context(), "example", func(_ context.Context, directory string) error {
		got = directory
		return nil
	}); err != nil || got != data {
		t.Fatalf("run = %v with %q, want %q", err, got, data)
	}
}
