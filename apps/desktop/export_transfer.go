package main

import (
	"crypto/rand"
	"errors"
	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"io"
	"sync"
)

type exportDestinationPicker interface{ ChooseFileDestination(string) (string, error) }

type exportTransfer struct {
	mu      sync.Mutex
	id      string
	pending *exportFile
	size    int
}

func (s *exportTransfer) abort() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discard()
}
func (s *exportTransfer) discard() {
	if s.pending != nil {
		s.pending.discard()
	}
	s.pending = nil
	s.id = ""
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
		s.discard()
		return "", err
	}
	f, err := prepareExport(destination)
	if err != nil {
		s.discard()
		return "", err
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
	position, err := s.pending.file.Seek(0, io.SeekCurrent)
	if err != nil {
		s.discard()
		return err
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
	if _, err := s.pending.file.Write(contents); err != nil {
		s.discard()
		return err
	}
	return nil
}
func (s *exportTransfer) finish(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || id != s.id {
		return errors.New("export does not belong to this transfer")
	}
	defer s.discard()
	position, err := s.pending.file.Seek(0, io.SeekCurrent)
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
		s.discard()
	}
	return nil
}
