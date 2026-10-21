package guardrail

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/exactint"
)

var (
	ErrInvalidPolicy    = errors.New("guardrails: invalid policy")
	ErrRevisionConflict = errors.New("guardrails: policy revision changed")
	ErrRevisionLimit    = errors.New("guardrails: policy revision exhausted")
	ErrProtectedContent = errors.New("guardrails: request contains protected content")
)

type Snapshot struct {
	Revision int64    `json:"revision"`
	Phrases  []string `json:"phrases"`
}

type Summary struct {
	Revision             int64 `json:"revision" jsonschema:"minimum=1"`
	ProtectedPhraseCount int   `json:"protectedPhraseCount" jsonschema:"minimum=0,maximum=32"`
}

type Policy struct {
	revision int64
	phrases  []string
}

func NewPolicy() Policy { return Policy{revision: 1, phrases: []string{}} }

func Restore(snapshot Snapshot) (Policy, error) {
	if snapshot.Revision < 1 || snapshot.Revision > exactint.Maximum || snapshot.Phrases == nil || len(snapshot.Phrases) > 32 {
		return Policy{}, ErrInvalidPolicy
	}
	seen := make(map[string]bool, len(snapshot.Phrases))
	for _, phrase := range snapshot.Phrases {
		if !utf8.ValidString(phrase) || len(phrase) > 512 || strings.TrimSpace(phrase) == "" || seen[phrase] {
			return Policy{}, ErrInvalidPolicy
		}
		seen[phrase] = true
	}
	return Policy{revision: snapshot.Revision, phrases: slices.Clone(snapshot.Phrases)}, nil
}

func (p Policy) Snapshot() Snapshot {
	return Snapshot{Revision: p.revision, Phrases: slices.Clone(p.phrases)}
}

func (p Policy) Summary() Summary {
	return Summary{Revision: p.revision, ProtectedPhraseCount: len(p.phrases)}
}

func (p Policy) Replace(expected int64, phrases []string) (Policy, error) {
	if p.revision < 1 {
		return Policy{}, ErrInvalidPolicy
	}
	if expected != p.revision {
		return Policy{}, ErrRevisionConflict
	}
	if p.revision == exactint.Maximum {
		return Policy{}, ErrRevisionLimit
	}
	return Restore(Snapshot{Revision: p.revision + 1, Phrases: phrases})
}

func (p Policy) Admit(text string) error {
	if p.revision < 1 {
		return ErrInvalidPolicy
	}
	for _, phrase := range p.phrases {
		if strings.Contains(text, phrase) {
			return ErrProtectedContent
		}
	}
	return nil
}
