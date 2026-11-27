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
	current *exportAttempt
}

type exportAttempt struct {
	id   string
	size int
	file *exportFile
}

var errUnownedExport = errors.New("export does not belong to this transfer")

func (s *exportTransfer) abort() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discard()
}

func (s *exportTransfer) discard() error {
	var err error
	if s.current != nil && s.current.file != nil {
		err = s.current.file.discard()
		if s.current.file.ownsStagingFile() {
			return err
		}
	}
	s.current = nil
	return err
}

func (s *exportTransfer) begin(name string, size int, picker exportDestinationPicker) (string, error) {
	if err := bridge.ValidateFileName(name); err != nil {
		return "", err
	}
	if size < 0 {
		return "", errors.New("export size cannot be negative")
	}
	s.mu.Lock()
	if s.current != nil {
		s.mu.Unlock()
		return "", errors.New("another file export is in progress")
	}
	attempt := &exportAttempt{id: rand.Text(), size: size}
	s.current = attempt
	s.mu.Unlock()
	// Navigation can revoke the originating document while the native picker is open.
	destination, err := picker.ChooseFileDestination(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != attempt {
		return "", nil
	}
	if err != nil || destination == "" {
		s.current = nil
		return "", err
	}
	attempt.file, err = prepareExport(destination)
	if err != nil {
		s.current = nil
		return "", err
	}
	return attempt.id, nil
}

func (s *exportTransfer) stagedAttempt(id string) *exportAttempt {
	if s.current != nil && s.current.id == id && s.current.file != nil {
		return s.current
	}
	return nil
}

func (s *exportTransfer) appendChunk(id string, offset int, encoded string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.stagedAttempt(id)
	if attempt == nil {
		return errUnownedExport
	}
	position, err := attempt.file.position()
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
	if len(contents) == 0 || len(contents) > attempt.size-offset {
		return errors.New("export chunk exceeds remaining bytes")
	}
	if _, err := attempt.file.Write(contents); err != nil {
		return errors.Join(err, s.discard())
	}
	return nil
}

func (s *exportTransfer) finish(id string) (returned error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.stagedAttempt(id)
	if attempt == nil {
		return errUnownedExport
	}
	defer func() { returned = errors.Join(returned, s.discard()) }()
	position, err := attempt.file.position()
	if err != nil {
		return err
	}
	if position != int64(attempt.size) {
		return errors.New("export is incomplete")
	}
	return attempt.file.publish()
}

func (s *exportTransfer) cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stagedAttempt(id) != nil {
		return s.discard()
	}
	return nil
}
