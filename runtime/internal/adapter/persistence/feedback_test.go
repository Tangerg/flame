package persistence

import (
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/annotation"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/jsonvalue"
)

func TestAnnotationRetryKeepsOriginalSourceAndTimestamp(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := annotationStore{db: db}
	content, _ := jsonvalue.Parse([]byte(`{"rating":"negative"}`))
	original := annotation.Entry{CommandID: "first", Subject: annotation.Subject{SessionID: "ses_time"}, Source: extension.AnnotationSource{ContributionSource: extension.ContributionSource{InstallationID: "f630f241-28b4-4fba-89bb-ddac2710a683", Release: integrity.Hash([]byte("first")).String(), Contribution: "notes"}, ContractReference: "test.annotations.v1"}, Content: content, CreatedAt: time.Unix(1, 123456789).UTC()}
	if err := store.Append(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	retry := original
	retry.CreatedAt = original.CreatedAt.Add(time.Second)
	if err := store.Append(t.Context(), retry); err != nil {
		t.Fatal(err)
	}
	retry.Source.Release = integrity.Hash([]byte("other")).String()
	if err := store.Append(t.Context(), retry); !errors.Is(err, annotation.ErrConflict) {
		t.Fatalf("changed source: %v", err)
	}
	got, err := store.ListSession(t.Context(), "ses_time")
	if err != nil || len(got) != 1 || got[0] != original {
		t.Fatalf("%+v, %v", got, err)
	}
}
