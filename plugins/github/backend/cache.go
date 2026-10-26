package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
)

const maxCacheBytes = 64 << 20

type response struct {
	body        []byte
	etag, links string
}
type cache struct{ db *sql.DB }

func openCache(ctx context.Context, path string) (*cache, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{`CREATE TABLE IF NOT EXISTS responses (key TEXT PRIMARY KEY, etag TEXT NOT NULL, links TEXT NOT NULL, body BLOB NOT NULL, checked_at INTEGER NOT NULL)`})
	if err != nil {
		return nil, err
	}
	return &cache{db: db}, nil
}

func (c *cache) read(ctx context.Context, key string) (response, error) {
	var result response
	err := sqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT etag,links,body FROM responses WHERE key=?`, key).Scan(&result.etag, &result.links, &result.body)
	if errors.Is(err, sql.ErrNoRows) {
		return response{}, nil
	}
	if err != nil {
		return response{}, err
	}
	if result.etag == "" || len(result.body) == 0 {
		return response{}, errors.New("github: invalid retained conditional response")
	}
	return result, nil
}

func (c *cache) save(ctx context.Context, key string, result response) error {
	return sqlite.RunInTx(ctx, c.db, func(ctx context.Context) error {
		connection := sqlite.Connection(ctx, c.db)
		if result.etag == "" {
			_, err := connection.ExecContext(ctx, `DELETE FROM responses WHERE key=?`, key)
			return err
		}
		if _, err := connection.ExecContext(ctx, `INSERT INTO responses(key,etag,links,body,checked_at) VALUES(?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET etag=excluded.etag,links=excluded.links,body=excluded.body,checked_at=excluded.checked_at`, key, result.etag, result.links, result.body, time.Now().UnixNano()); err != nil {
			return err
		}
		// Cache eviction releases projections; it can never retire a business receipt.
		_, err := connection.ExecContext(ctx, `DELETE FROM responses WHERE key IN (SELECT key FROM (SELECT key,SUM(length(body)+length(etag)+length(links)+length(key)) OVER (ORDER BY checked_at DESC,key) AS retained_bytes FROM responses) WHERE retained_bytes>?)`, maxCacheBytes)
		return err
	})
}
