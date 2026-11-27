package workbench

import (
	"bytes"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
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

func (m *memoryStorage) Keys(prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for name := range m.records {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (m *memoryStorage) Commit(changes ...RecordChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, change := range changes {
		if change.Body == nil {
			delete(m.records, change.Key)
		} else {
			m.records[change.Key] = bytes.Clone(change.Body)
		}
	}
	return nil
}

func (m *memoryStorage) Close() error { return nil }
