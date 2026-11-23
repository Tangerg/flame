package workbench

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem/commit"
)

// Records live as long as the process: that is all the durability a
// workbench without a state directory promises.
type memoryStorage struct {
	mu      sync.Mutex
	records map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{records: make(map[string][]byte)}
}

func (m *memoryStorage) Read(name string, maximumBytes int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, found := m.records[name]
	if !found {
		return nil, fs.ErrNotExist
	}
	if int64(len(body)) > maximumBytes {
		return nil, fmt.Errorf("workbench record %q exceeds %d bytes", name, maximumBytes)
	}
	return bytes.Clone(body), nil
}

func (m *memoryStorage) ListFiles(directory, extension string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for name := range m.records {
		if filepath.Dir(name) == directory && filepath.Ext(name) == extension {
			names = append(names, filepath.Base(name))
		}
	}
	slices.Sort(names)
	return names, nil
}

func (m *memoryStorage) Replace(name string, body []byte) commit.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[name] = bytes.Clone(body)
	return commit.Result{State: commit.Durable}
}

func (m *memoryStorage) Remove(name string) commit.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, name)
	return commit.Result{State: commit.Durable}
}

func (m *memoryStorage) ConfirmDurability() error { return nil }
