package agentmemory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

type FactBatch struct {
	Project    string
	SessionID  identity.SessionID
	Day        string
	Facts      []string
	CapturedAt time.Time
}

func (f FactBatch) Normalize() (FactBatch, error) {
	f.Project = strings.TrimSpace(f.Project)
	if f.Project == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch project is required")
	}
	if f.SessionID.IsZero() {
		return FactBatch{}, errors.New("agentmemory: fact batch session is required")
	}
	day, err := time.Parse(time.DateOnly, f.Day)
	if err != nil || day.Format(time.DateOnly) != f.Day {
		return FactBatch{}, fmt.Errorf("agentmemory: invalid ledger day %q", f.Day)
	}
	if f.CapturedAt.IsZero() {
		return FactBatch{}, errors.New("agentmemory: fact batch capture time is required")
	}
	if err := instant.Validate(f.CapturedAt); err != nil {
		return FactBatch{}, fmt.Errorf("agentmemory: fact batch capture time: %w", err)
	}
	f.Facts, err = normalizeFactList(f.Facts, MaxFactsPerBatch, "fact batch")
	if err != nil {
		return FactBatch{}, err
	}
	return f, nil
}

type LedgerFact struct {
	Sequence   int64
	Day        string
	Content    string
	CapturedAt time.Time
}

func (l LedgerFact) Validate() error {
	if l.Sequence <= 0 {
		return errors.New("agentmemory: ledger fact sequence must be positive")
	}
	day, err := time.Parse(time.DateOnly, l.Day)
	if err != nil || day.Format(time.DateOnly) != l.Day {
		return fmt.Errorf("agentmemory: invalid ledger day %q", l.Day)
	}
	content, err := NormalizeContent(l.Content)
	if err != nil {
		return fmt.Errorf("agentmemory: ledger fact content: %w", err)
	}
	if content != l.Content {
		return errors.New("agentmemory: ledger fact content is not canonical")
	}
	if l.CapturedAt.IsZero() {
		return errors.New("agentmemory: ledger fact capture time is required")
	}
	return instant.Validate(l.CapturedAt)
}

type State struct {
	Watermark int64
	UpdatedAt time.Time
}

func (s State) Validate() error {
	if s.Watermark < 0 {
		return errors.New("agentmemory: curation watermark must not be negative")
	}
	if s.Watermark == 0 && !s.UpdatedAt.IsZero() {
		return errors.New("agentmemory: empty curation state carries an update time")
	}
	if s.Watermark > 0 && s.UpdatedAt.IsZero() {
		return errors.New("agentmemory: advanced curation state has no update time")
	}
	return instant.Validate(s.UpdatedAt)
}

func normalizeFactList(input []string, maximum int, collection string) ([]string, error) {
	if len(input) > maximum {
		return nil, fmt.Errorf("%w: %s exceeds %d items", ErrInvalidFacts, collection, maximum)
	}
	var normalized []string
	seen := make(map[string]struct{})
	for _, fact := range input {
		var err error
		fact, err = NormalizeContent(fact)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid %s item: %w", ErrInvalidFacts, collection, err)
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		normalized = append(normalized, fact)
	}
	return normalized, nil
}
