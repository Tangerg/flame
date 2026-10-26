// Package persistencefixture owns product stores and raw SQL observations for
// cross-package integration tests. Persistence white-box tests use their owner directly.
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

// Stores keeps the product write-set on Bundle. The separate SQL connection is
// only for deliberate corruption injection and raw evidence assertions.
type Stores struct {
	*persistence.Bundle
	*sql.DB
}

func Open(tb testing.TB, ctx context.Context, dataDirectory string) (*Stores, error) {
	tb.Helper()
	bundle, err := persistence.Open(ctx, persistence.Config{DataDirectory: dataDirectory})
	if err != nil {
		return nil, err
	}
	layout, err := localruntime.DataDirectoryAt(bundle.DataDirectory)
	if err != nil {
		return nil, errors.Join(err, bundle.Close())
	}
	address := &url.URL{Scheme: "file", Path: layout.DatabasePath()}
	query := address.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	address.RawQuery = query.Encode()
	inspection, err := sql.Open("sqlite", address.String())
	if err != nil {
		return nil, errors.Join(err, bundle.Close())
	}
	return &Stores{Bundle: bundle, DB: inspection}, nil
}

func (s *Stores) Close() error { return errors.Join(s.Bundle.Close(), s.DB.Close()) }
