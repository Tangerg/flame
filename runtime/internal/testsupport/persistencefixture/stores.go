package persistencefixture

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"testing"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
)

type Stores struct {
	*persistence.Bundle
	*sql.DB
}

func Open(tb testing.TB, ctx context.Context, dataDirectory string) (*Stores, error) {
	tb.Helper()
	bundle, err := persistence.Open(ctx, persistence.Config{DataDirectory: dataDirectory, ValidateDirectory: UnleasedDirectory})
	if err != nil {
		return nil, err
	}
	inspection, err := OpenRawSQL(bundle.DataDirectory)
	if err != nil {
		return nil, errors.Join(err, bundle.Close())
	}
	return &Stores{Bundle: bundle, DB: inspection}, nil
}

func OpenRawSQL(dataDirectory string) (*sql.DB, error) {
	layout, err := localruntime.DataDirectoryAt(dataDirectory)
	if err != nil {
		return nil, err
	}
	address := &url.URL{Scheme: "file", Path: layout.DatabasePath()}
	query := address.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	address.RawQuery = query.Encode()
	return sql.Open("sqlite", address.String())
}

func (s *Stores) Close() error { return errors.Join(s.Bundle.Close(), s.DB.Close()) }

// A test that opens a data directory without holding the Runtime lease
// has no ownership to verify.
func UnleasedDirectory() error { return nil }
