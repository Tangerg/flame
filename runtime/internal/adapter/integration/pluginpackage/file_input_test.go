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
	cancel  func()
	failure error
}

func (r cancelingPackageInput) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	r.cancel()
	return n, errors.Join(err, r.failure)
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
	failure := errors.New("source failed during copy")
	input := cancelingPackageInput{Reader: strings.NewReader("package content"), cancel: func() { cancel(cause) }, failure: failure}
	if err := staged.writeFile(ctx, "resource.txt", 0600, input); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
		t.Fatalf("canceled copy lost a concurrent outcome: %v", err)
	}
}

func TestMaterializeRejectsCancellationBeforeStaging(t *testing.T) {
	r := testReleases(t)
	cause := errors.New("package admission retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if candidate, err := r.Materialize(ctx, t.TempDir()); candidate != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("materialize = %v, %v; want cancellation category and owner cause", candidate, err)
	}
	entries, err := os.ReadDir(r.directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("canceled admission left staging: %v, %v", entries, err)
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
