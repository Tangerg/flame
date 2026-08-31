package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

// ProviderStore persists only stored provider configuration. Effective
// environment credentials belong to the registry decorator and never reach
// this adapter. Nullable columns encode absence directly.
type ProviderStore struct {
	db *sql.DB
}

func NewProviderStore(db *sql.DB) *ProviderStore {
	return &ProviderStore{db: db}
}

func scanProvider(row scanRow) (provider.Provider, error) {
	var (
		id         string
		rawAPIKey  sql.NullString
		rawBaseURL sql.NullString
	)
	if err := row.Scan(&id, &rawAPIKey, &rawBaseURL); err != nil {
		return provider.Provider{}, err
	}
	entry, err := provider.New(id)
	if err != nil {
		return provider.Provider{}, fmt.Errorf("decode identity: %w", err)
	}
	patch := provider.Patch{}
	if rawAPIKey.Valid {
		key, keyErr := provider.NewAPIKey(rawAPIKey.String)
		if keyErr != nil {
			return provider.Provider{}, fmt.Errorf("decode credential: %w", keyErr)
		}
		patch.APIKey = provider.Set(key)
	}
	if rawBaseURL.Valid {
		baseURL, baseURLErr := provider.NewBaseURL(rawBaseURL.String)
		if baseURLErr != nil {
			return provider.Provider{}, fmt.Errorf("decode base URL: %w", baseURLErr)
		}
		patch.BaseURL = provider.Set(baseURL)
	}
	entry, err = entry.Apply(patch)
	if err != nil {
		return provider.Provider{}, fmt.Errorf("decode provider: %w", err)
	}
	return entry, nil
}

func (p *ProviderStore) List(ctx context.Context) ([]provider.Provider, error) {
	rows, err := conn(ctx, p.db).QueryContext(ctx,
		`SELECT id, api_key, base_url FROM providers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []provider.Provider
	for rows.Next() {
		entry, scanErr := scanProvider(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan provider: %w", scanErr)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	return out, nil
}

func (p *ProviderStore) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	entry, err := scanProvider(conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT id, api_key, base_url FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return provider.Provider{}, false, nil
	}
	if err != nil {
		return provider.Provider{}, false, fmt.Errorf("sqlite: get provider: %w", err)
	}
	return entry, true, nil
}

func storedProviderValues(entry provider.Provider) (apiKey, baseURL any) {
	if key, configured := entry.APIKey(); configured {
		apiKey = key.Reveal()
	}
	if endpoint, present := entry.BaseURL(); present {
		baseURL = endpoint.String()
	}
	return apiKey, baseURL
}

// Update serializes read/apply/write in one transaction so preserve/set/clear
// remains atomic under concurrent mutations without exposing Change internals
// to the storage adapter.
func (p *ProviderStore) Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	var updated provider.Provider
	err := RunInTx(ctx, p.db, func(txCtx context.Context) error {
		current, found, getErr := p.Get(txCtx, id)
		if getErr != nil {
			return getErr
		}
		if !found {
			var newErr error
			current, newErr = provider.New(id)
			if newErr != nil {
				return newErr
			}
		}
		var applyErr error
		updated, applyErr = current.Apply(patch)
		if applyErr != nil {
			return applyErr
		}
		apiKey, baseURL := storedProviderValues(updated)
		_, writeErr := conn(txCtx, p.db).ExecContext(txCtx,
			`INSERT INTO providers (id, api_key, base_url) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   api_key = excluded.api_key,
			   base_url = excluded.base_url`,
			updated.ID(), apiKey, baseURL,
		)
		return writeErr
	})
	if err != nil {
		return provider.Provider{}, fmt.Errorf("sqlite: update provider: %w", err)
	}
	return updated, nil
}
