package workbenchdb

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"modernc.org/sqlite"
)

const databaseName = "authoring.db"

const createRecords = `CREATE TABLE IF NOT EXISTS authoring_records (
	key TEXT PRIMARY KEY,
	body BLOB NOT NULL
) STRICT, WITHOUT ROWID`

type Store struct {
	database  *sql.DB
	lease     *filesystem.DirectoryLease
	directory string
	identity  os.FileInfo
}

func Open(directory string) (_ *Store, err error) {
	directory, err = filesystem.CanonicalPath(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve authoring directory: %w", err)
	}
	if err := os.MkdirAll(directory, filesystem.PrivateDirectoryMode); err != nil {
		return nil, fmt.Errorf("create authoring directory: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	identity, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	lease, err := filesystem.TryDirectoryLease(directory)
	if err != nil {
		return nil, fmt.Errorf("acquire authoring directory: %w", err)
	}
	if lease == nil {
		return nil, errors.New("authoring directory is already in use")
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, lease.Release())
		}
	}()
	current, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(identity, current) {
		return nil, errors.New("authoring directory changed during acquisition")
	}
	if err := prepareDatabaseFile(root); err != nil {
		return nil, err
	}
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(directory, databaseName))}
	query := address.Query()
	query.Set("mode", "rw")
	query.Set("_journal_mode", "DELETE")
	// The rollback journal's removal commits the transaction, so sync its directory too.
	query.Set("_synchronous", "EXTRA")
	address.RawQuery = query.Encode()
	connector, err := sqlite.NewConnector(address.String())
	if err != nil {
		return nil, err
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(createRecords); err != nil {
		return nil, errors.Join(fmt.Errorf("open authoring database: %w", err), database.Close())
	}
	return &Store{database: database, lease: lease, directory: directory, identity: identity}, nil
}

func prepareDatabaseFile(root *os.Root) error {
	info, err := root.Lstat(databaseName)
	if errors.Is(err, fs.ErrNotExist) {
		file, err := root.OpenFile(databaseName, os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystem.PrivateFileMode)
		if err != nil {
			return err
		}
		return file.Close()
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&^filesystem.PrivateFileMode != 0) {
		return errors.New("authoring database must be a private regular file")
	}
	return nil
}

func (s *Store) Close() error {
	return errors.Join(s.database.Close(), s.lease.Release())
}

func (s *Store) verifyDirectory() error {
	current, err := os.Stat(s.directory)
	if err != nil {
		return fmt.Errorf("inspect authoring directory: %w", err)
	}
	if !os.SameFile(s.identity, current) {
		return errors.New("authoring directory changed while in use")
	}
	return nil
}

func (s *Store) Read(key string, maximumBytes int64) ([]byte, error) {
	if err := s.verifyDirectory(); err != nil {
		return nil, err
	}
	var body []byte
	var size int64
	err := s.database.QueryRow(`
		SELECT length(body), CASE WHEN length(body) <= ? THEN body END
		FROM authoring_records WHERE key = ?`, maximumBytes, key).Scan(&size, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if size > maximumBytes {
		return nil, fmt.Errorf("authoring record %q exceeds %d bytes", key, maximumBytes)
	}
	return body, nil
}

func (s *Store) Keys(prefix string) ([]string, error) {
	if err := s.verifyDirectory(); err != nil {
		return nil, err
	}
	rows, err := s.database.Query(`
		SELECT key FROM authoring_records WHERE substr(key, 1, length(?)) = ? ORDER BY key`, prefix, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) Commit(changes ...workbench.RecordChange) (err error) {
	if err := s.verifyDirectory(); err != nil {
		return err
	}
	tx, err := s.database.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	for _, change := range changes {
		if change.Body == nil {
			_, err = tx.Exec(`DELETE FROM authoring_records WHERE key = ?`, change.Key)
		} else {
			_, err = tx.Exec(`INSERT INTO authoring_records (key, body) VALUES (?, ?)
				ON CONFLICT (key) DO UPDATE SET body = excluded.body`, change.Key, change.Body)
		}
		if err != nil {
			return fmt.Errorf("write authoring record %q: %w", change.Key, err)
		}
	}
	return tx.Commit()
}
