package skillauthoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func digestDocument(content []byte, exists bool) string {
	if !exists {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (s *Store) Preimage(ctx context.Context, proposal skills.Proposal) (string, error) {
	if err := s.validatePublication(proposal); err != nil {
		return "", err
	}
	root, close, err := s.openLeasedRoot(ctx, "inspect publication target")
	if err != nil {
		return "", err
	}
	defer close()
	content, exists, err := readSkill(root, s.activeDir(proposal.Name))
	if err != nil {
		return "", err
	}
	if exists && !proposal.Revises {
		return "", skills.ErrConflict
	}
	return digestDocument(content, exists), nil
}
func (s *Store) validatePublication(p skills.Proposal) error {
	if p.Scope != s.scope {
		return skills.ErrConflict
	}
	return p.Validate()
}
