package skillauthoring

import (
	"context"
	"crypto/rand"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	skillspec "github.com/Tangerg/scope/skills"
)

const (
	usageFile             = ".usage.json"
	maxUsageMetadataBytes = 64 << 10
)

type usageRecord struct {
	FirstSeen time.Time `json:"firstSeen"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

func (u usageRecord) lastActivity() time.Time {
	if u.LastUsed.After(u.FirstSeen) {
		return u.LastUsed
	}
	return u.FirstSeen
}

func (s *Store) RecordUse(ctx context.Context, name string, now time.Time) error {
	if err := contextError(ctx, "record skill use"); err != nil {
		return err
	}
	if !validName(name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "record skill use")
	if err != nil {
		return err
	}
	defer cleanup()

	usage, err := readUsage(ctx, root)
	if err != nil {
		return err
	}
	record := usage[name]
	ts := now.UTC()
	if record.FirstSeen.IsZero() {
		record.FirstSeen = ts
		record.LastUsed = ts
	} else if ts.After(record.LastUsed) {
		record.LastUsed = ts
	}
	usage[name] = record
	return writeUsage(ctx, root, usage)
}

func (s *Store) SweepIdle(ctx context.Context, now time.Time, inactiveSince time.Time) ([]string, []string, error) {
	if err := contextError(ctx, "sweep skills"); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "sweep skills")
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	names, err := activeSkillNames(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	usage, err := readUsage(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	var archived []string
	var identities []string
	for _, name := range names {
		content, found, err := readSkill(root, name)
		if err != nil {
			return archived, identities, err
		}
		if !found {
			continue
		}
		skill, err := skilldocument.Load(ctx, name, content)
		if errors.Is(err, skillspec.ErrInvalidSkill) {
			continue
		}
		if err != nil {
			return archived, identities, fmt.Errorf("skillauthoring: load idle skill %q: %w", name, err)
		}
		if skills.ProposalOrigin(skill.Metadata[metadataOrigin]).Validate() != nil {
			continue
		}
		record := usage[name]
		if record.FirstSeen.IsZero() {
			record.FirstSeen = now.UTC()
		}
		if !record.lastActivity().After(inactiveSince) {
			changed, err := s.archiveActive(root, name)
			identities = append(identities, changed...)
			if err != nil {
				return archived, distinctPaths(identities), err
			}
			delete(usage, name)
			archived = append(archived, name)
			continue
		}
		usage[name] = record
	}
	if err := writeUsage(ctx, root, usage); err != nil {
		return archived, distinctPaths(identities), err
	}
	return archived, distinctPaths(identities), nil
}

func activeSkillNames(ctx context.Context, root *os.Root) ([]string, error) {
	if err := contextError(ctx, "list active skills"); err != nil {
		return nil, err
	}
	active, _, err := managedSkillNames(root)
	return active, err
}

func readUsageFile(ctx context.Context, root *os.Root) ([]byte, bool, error) {
	file, opened, err := filesystem.OpenFileAt(root, usageFile, maxUsageMetadataBytes)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, false, nil
	case errors.Is(err, filesystem.ErrNotRegular):
		return nil, false, fmt.Errorf("skillauthoring: usage metadata admission: %w", err)
	case errors.Is(err, filesystem.ErrFileTooLarge):
		return nil, false, fmt.Errorf("%w: metadata admission: %w", skills.ErrUsageTooLarge, err)
	case err != nil:
		return nil, false, fmt.Errorf("skillauthoring: open usage: %w", err)
	}
	contextReader, err := capture.NewReader(ctx, file)
	if err != nil {
		return nil, false, errors.Join(err, file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader, maxUsageMetadataBytes+1))
	verifyErr := filesystem.VerifyAtVersion(file, opened, root, usageFile)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, false, fmt.Errorf("skillauthoring: read usage: %w", errors.Join(readErr, verifyErr, closeErr))
	}
	if len(data) > maxUsageMetadataBytes {
		return nil, false, fmt.Errorf("%w: exceeds %d bytes", skills.ErrUsageTooLarge, maxUsageMetadataBytes)
	}
	if verifyErr != nil {
		return nil, false, fmt.Errorf("skillauthoring: read usage: %w", verifyErr)
	}
	return data, true, nil
}

func readUsage(ctx context.Context, root *os.Root) (map[string]usageRecord, error) {
	if err := contextError(ctx, "read skill usage"); err != nil {
		return nil, err
	}
	data, present, err := readUsageFile(ctx, root)
	if err != nil {
		return nil, err
	}
	if !present {
		return map[string]usageRecord{}, nil
	}
	var usage map[string]usageRecord
	if err := json.Unmarshal(data, &usage, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("skillauthoring: decode usage: %w", err)
	}
	if usage == nil {
		return nil, errors.New("skillauthoring: usage metadata must be an object")
	}
	if len(usage) > skills.MaxSkillsPerSource {
		return nil, fmt.Errorf(
			"%w: usage metadata contains %d records; limit is %d",
			skills.ErrLibraryCapacity,
			len(usage),
			skills.MaxSkillsPerSource,
		)
	}
	for name := range usage {
		if !validName(name) {
			return nil, fmt.Errorf("skillauthoring: usage metadata has invalid skill name %q", name)
		}
	}
	return usage, nil
}

func writeUsage(ctx context.Context, root *os.Root, usage map[string]usageRecord) error {
	if err := contextError(ctx, "write skill usage"); err != nil {
		return err
	}
	if len(usage) > skills.MaxSkillsPerSource {
		return fmt.Errorf(
			"%w: cannot write %d usage records; limit is %d",
			skills.ErrLibraryCapacity,
			len(usage),
			skills.MaxSkillsPerSource,
		)
	}
	data, err := json.Marshal(usage, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("skillauthoring: marshal usage: %w", err)
	}
	if len(data) > maxUsageMetadataBytes {
		return fmt.Errorf("%w: encoded usage is %d bytes; limit is %d", skills.ErrUsageTooLarge, len(data), maxUsageMetadataBytes)
	}
	temporary := usageFile + ".tmp-" + rand.Text()
	if err := contextError(ctx, "write skill usage"); err != nil {
		return err
	}
	if err := writeFile(root, temporary, data); err != nil {
		return err
	}
	if err := contextError(ctx, "commit skill usage"); err != nil {
		_ = root.Remove(temporary)
		return err
	}
	if err := root.Rename(temporary, usageFile); err != nil {
		_ = root.Remove(temporary)
		return fmt.Errorf("skillauthoring: commit usage: %w", err)
	}
	return nil
}
