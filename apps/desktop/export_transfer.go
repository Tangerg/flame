package main

import (
	"crypto/rand"
	"errors"
	"sync"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
)

type exportDestinationPicker interface{ ChooseFileDestination(string) (string, error) }

type exportTransfer struct {
	mu      sync.Mutex
	id      string
	pending *exportFile
	size    int
}

func (s *exportTransfer) abort() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discard()
}
func (s *exportTransfer) discard() error {
	var err error
	if s.pending != nil {
		err = s.pending.discard()
		if s.pending.ownsStagingFile() {
			return err
		}
	}
	s.pending = nil
	s.id = ""
	s.size = 0
	return err
}

func (s *exportTransfer) begin(name string, size int, picker exportDestinationPicker) (string, error) {
	if err := bridge.ValidateFileName(name); err != nil {
		return "", err
	}
	if size <= 0 {
		return "", errors.New("export size must be positive")
	}
	id := rand.Text()
	s.mu.Lock()
	if s.id != "" {
		s.mu.Unlock()
		return "", errors.New("another file export is in progress")
	}
	s.id = id
	s.mu.Unlock()
	// Navigation can revoke the originating document while the native picker is open.
	destination, err := picker.ChooseFileDestination(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != id {
		return "", nil
	}
	if err != nil || destination == "" {
		return "", errors.Join(err, s.discard())
	}
	f, err := prepareExport(destination)
	if err != nil {
		return "", errors.Join(err, s.discard())
	}
	s.pending = f
	s.size = size
	return id, nil
}
func (s *exportTransfer) appendChunk(id string, offset int, encoded string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || id != s.id {
		return errors.New("export does not belong to this transfer")
	}
	position, err := s.pending.position()
	if err != nil {
		return errors.Join(err, s.discard())
	}
	if int64(offset) != position {
		return errors.New("export offset does not match written bytes")
	}
	contents, err := bridge.DecodeExportChunk(encoded)
	if err != nil {
		return err
	}
	if len(contents) == 0 || len(contents) > s.size-offset {
		return errors.New("export chunk exceeds remaining bytes")
	}
	if _, err := s.pending.Write(contents); err != nil {
		return errors.Join(err, s.discard())
	}
	return nil
}
func (s *exportTransfer) finish(id string) (returned error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || id != s.id {
		return errors.New("export does not belong to this transfer")
	}
	defer func() { returned = errors.Join(returned, s.discard()) }()
	position, err := s.pending.position()
	if err != nil {
		return err
	}
	if position != int64(s.size) {
		return errors.New("export is incomplete")
	}
	return s.pending.publish()
}
func (s *exportTransfer) cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != nil && id == s.id {
		return s.discard()
	}
	return nil
}
