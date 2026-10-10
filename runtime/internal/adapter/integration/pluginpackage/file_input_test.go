package pluginpackage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

type cancelingPackageInput struct {
	io.Reader
	cancel func()
}

func (r cancelingPackageInput) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	r.cancel()
	return n, err
}

func TestPackageCopyCannotSucceedAfterReadCancellation(t *testing.T) {
	staged, err := newStaging(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := staged.discard(); err != nil {
			t.Error(err)
		}
	})
	cause := errors.New("installation preparation withdrawn")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	input := cancelingPackageInput{Reader: strings.NewReader("package content"), cancel: func() { cancel(cause) }}
	if err := staged.writeFile(ctx, "resource.txt", 0600, input); !errors.Is(err, cause) {
		t.Fatalf("canceled copy lost its outcome: %v", err)
	}
}

func TestArchiveAdmissionRefusesAReplacedSource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "package.zip")
	if err := os.WriteFile(source, nil, 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(source), "replacement.zip")
	if err := os.WriteFile(replacement, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, source); err != nil {
		t.Fatal(err)
	}
	staged, err := newStaging(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := staged.discard(); err != nil {
			t.Error(err)
		}
	})
	err = staged.extractArchive(t.Context(), source, expected)
	if !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, fileinput.ErrChanged) {
		t.Fatalf("replaced archive lost its identity failure: %v", err)
	}
}
