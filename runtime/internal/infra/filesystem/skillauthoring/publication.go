package skillauthoring

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func (s *Store) Publish(ctx context.Context, proposal skills.Proposal, previous, permit, journal string, admit func(context.Context, func(context.Context) error) error) ([]string, error) {
	if admit == nil {
		return nil, errors.New("skill publication: original authority is required")
	}
	if err := s.validatePublication(proposal); err != nil {
		return nil, err
	}
	if err := (skills.ProposalRef{Scope: proposal.Scope, Name: proposal.Name, Revision: permit}).Validate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(journal) {
		return nil, errors.New("skill publication: private journal is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, release, err := s.openWritableRoot(ctx, "publish approved bytes")
	if err != nil {
		return nil, err
	}
	defer release()
	p := publication{store: s, root: root, proposal: proposal, previous: previous, permit: permit}
	// Skill moves also take the library lease before entering Frame admission.
	var changed []string
	err = admit(ctx, func(ctx context.Context) error {
		var err error
		changed, err = p.commit(ctx, journal)
		return err
	})
	return changed, err
}

type publication struct {
	store            *Store
	root             *os.Root
	proposal         skills.Proposal
	previous, permit string
}

func (p publication) binding() (string, error) {
	data, err := json.Marshal(struct {
		Library  string
		Proposal skills.Proposal
		Previous string
	}{p.store.root, p.proposal, p.previous}, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func (p publication) commit(ctx context.Context, journal string) (changed []string, err error) {
	if err := os.MkdirAll(journal, filesystem.PrivateDirectoryMode); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(journal)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	receipt := publicationReceipt{root: root, permit: p.permit}
	binding, err := p.binding()
	if err != nil {
		return nil, err
	}
	record, found, err := receipt.load()
	if err != nil {
		return nil, err
	}
	if found && record.Binding != binding {
		return nil, skills.ErrProposalChanged
	}
	if !found {
		proof, err := p.prepare(ctx)
		if err != nil {
			return nil, p.settle(ctx, receipt, binding, err)
		}
		record = publicationRecord{Binding: binding, State: publicationPrepared, Proof: &proof}
		if err := receipt.save(record); err != nil {
			return nil, err
		}
	}
	return p.apply(ctx, receipt, record)
}

func (p publication) apply(ctx context.Context, receipt publicationReceipt, record publicationRecord) ([]string, error) {
	switch record.State {
	case publicationPublished:
		p.cleanStage(ctx)
		return nil, nil
	case publicationRefused:
		p.cleanStage(ctx)
		return nil, skills.ErrPublicationRefused
	}
	changed, err := p.place(ctx, *record.Proof)
	return changed, p.settle(ctx, receipt, record.Binding, err)
}

func (p publication) settle(ctx context.Context, receipt publicationReceipt, binding string, cause error) error {
	state := publicationPublished
	if cause != nil {
		if !knownPublicationRefusal(cause) {
			return cause
		}
		state = publicationRefused
	}
	if err := receipt.save(publicationRecord{Binding: binding, State: state}); err != nil {
		return errors.Join(skills.ErrPublicationUnconfirmed, cause, err)
	}
	p.cleanStage(ctx)
	if state == publicationRefused {
		return errors.Join(skills.ErrPublicationRefused, cause)
	}
	return cause
}

func knownPublicationRefusal(err error) bool {
	return errors.Is(err, skills.ErrConflict) || errors.Is(err, skills.ErrProposalChanged) || errors.Is(err, skills.ErrLibraryCapacity)
}

func (p publication) cleanStage(ctx context.Context) {
	if err := p.root.RemoveAll(p.stagedDirectory()); err != nil {
		slog.ErrorContext(ctx, "skill publication: remove retired staging directory", slog.String("skill.name", p.proposal.Name), telemetry.Error(err))
	}
}
