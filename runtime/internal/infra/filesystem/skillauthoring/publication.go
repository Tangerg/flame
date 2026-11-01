package skillauthoring

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func (s *Store) Publish(ctx context.Context, proposal skills.Proposal, previous, permit, journal string) ([]string, error) {
	if previous != "" && !proposal.Revises {
		return nil, skills.ErrConflict
	}
	if err := s.validatePublication(proposal); err != nil {
		return nil, err
	}
	if err := (skills.ProposalRef{Scope: proposal.Scope, Name: proposal.Name, Revision: permit}).Validate(); err != nil {
		return nil, err
	}
	content, err := renderProposal(proposal)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, close, err := s.openLeasedRoot(ctx, "publish approved bytes")
	if err != nil {
		return nil, err
	}
	defer close()
	p := publication{store: s, root: root, name: proposal.Name, content: content, previous: previous}
	return p.commit(ctx, journal, permit)
}

type publication struct {
	store          *Store
	root           *os.Root
	name, previous string
	content        []byte
}

func (p publication) commit(ctx context.Context, journal, permit string) ([]string, error) {
	if !filepath.IsAbs(journal) {
		return nil, errors.New("skill publication: private journal is required")
	}
	if err := os.MkdirAll(journal, 0700); err != nil {
		return nil, err
	}
	records, err := os.OpenRoot(journal)
	if err != nil {
		return nil, err
	}
	defer records.Close()
	receipt := publicationReceipt{root: records, permit: permit}
	fresh, err := receipt.claim(p)
	if err != nil {
		return nil, err
	}
	if fresh {
		identities, err := p.publish(ctx)
		return identities, receipt.settle(err)
	}
	if done, err := receipt.has(publishedMarker); done || err != nil {
		return nil, err
	}
	if refused, err := receipt.has(refusedMarker); err != nil {
		return nil, err
	} else if refused {
		return nil, skills.ErrProposalChanged
	}
	return nil, receipt.settle(p.reconcile())
}

// A workspace can remove or forge its own files. Recovery only settles a
// pending host intent from the exact approved bytes. A later user edit leaves
// the outcome uncertain and never authorizes another write.
func (p publication) reconcile() error {
	active, exists, err := readSkill(p.root, p.name)
	if err != nil {
		return err
	}
	if exists && bytes.Equal(active, p.content) {
		return nil
	}
	return skills.ErrPublicationUnconfirmed
}

func (p publication) publish(ctx context.Context) ([]string, error) {
	identities, err := p.prepareDestination()
	if err != nil {
		return identities, err
	}
	if err := ctx.Err(); err != nil {
		return identities, err
	}
	if err := stageSkill(ctx, p.root, p.name, p.content); err != nil {
		return identities, err
	}
	return distinctPaths(append(identities, p.store.skillIdentities(p.name)...)), nil
}

func (p publication) prepareDestination() ([]string, error) {
	active, exists, err := readSkill(p.root, p.name)
	if err != nil {
		return nil, err
	}
	archived, hasArchive, err := readSkill(p.root, p.store.archiveDir(p.name))
	if err != nil {
		return nil, err
	}
	if !exists && p.previous != "" && digestDocument(archived, hasArchive) == p.previous {
		return nil, nil
	}
	if digestDocument(active, exists) != p.previous {
		return nil, skills.ErrProposalChanged
	}
	if !exists && hasArchive {
		return nil, skills.ErrConflict
	}
	if !hasArchive {
		if err := ensureManagedSkillCapacity(p.root); err != nil {
			return nil, err
		}
	}
	if exists {
		return p.store.archiveActive(p.root, p.name)
	}
	if _, err := p.root.Lstat(p.name); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return nil, err
		}
		return nil, skills.ErrConflict
	}
	return nil, nil
}
