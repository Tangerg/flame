package skillauthoring

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	skillspec "github.com/Tangerg/scope/skills"
)

const publicationStagePrefix = ".skill-publication-"

func (p publication) stagedDirectory() string { return publicationStagePrefix + p.permit }

func (p publication) activeDocument() string {
	return filepath.Join(p.proposal.Name, skillspec.SkillFile)
}

func (p publication) preimage() ([]byte, error) {
	if p.proposal.Revises != (p.previous != "") {
		return nil, skills.ErrProposalChanged
	}
	name := p.proposal.Name
	active, exists, err := readSkill(p.root, name)
	if err != nil {
		return nil, err
	}
	if digestDocument(active, exists) != p.previous {
		return nil, skills.ErrProposalChanged
	}
	if err := requireVacantDirectory(p.root, p.store.archiveDir(name)); err != nil {
		return nil, err
	}
	if p.proposal.Revises {
		info, err := p.root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, skills.ErrConflict
		}
		return active, nil
	}
	if err := requireVacantDirectory(p.root, name); err != nil {
		return nil, err
	}
	return nil, ensureManagedSkillCapacity(p.root)
}

func (p publication) render(ctx context.Context, original []byte) ([]byte, fs.FileMode, error) {
	frontmatter, mode := proposalFrontmatter(p.proposal), filesystem.SharedFileMode
	if p.proposal.Revises {
		skill, err := skilldocument.Load(ctx, p.proposal.Name, original)
		if err != nil {
			return nil, 0, err
		}
		info, err := p.root.Stat(p.activeDocument())
		if err != nil {
			return nil, 0, err
		}
		frontmatter, mode = skill.Frontmatter, info.Mode().Perm()
		frontmatter.Description = p.proposal.Description
	}
	content, err := renderDocument(frontmatter, p.proposal.Instructions)
	return content, mode, err
}

func (p publication) prepare(ctx context.Context) (publicationProof, error) {
	original, err := p.preimage()
	if err != nil {
		return publicationProof{}, err
	}
	content, mode, err := p.render(ctx, original)
	if err != nil {
		return publicationProof{}, err
	}
	if err := ctx.Err(); err != nil {
		return publicationProof{}, err
	}
	directory := p.stagedDirectory()
	if err := p.root.RemoveAll(directory); err != nil {
		return publicationProof{}, err
	}
	if err := p.root.Mkdir(directory, filesystem.SharedDirectoryMode); err != nil {
		return publicationProof{}, err
	}
	document := filepath.Join(directory, skillspec.SkillFile)
	if err := writeFile(p.root, document, content, mode); err != nil {
		return publicationProof{}, errors.Join(err, p.root.RemoveAll(directory))
	}
	id, err := documentFileID(p.root, document)
	if err != nil {
		return publicationProof{}, errors.Join(err, p.root.RemoveAll(directory))
	}
	if id == "" {
		return publicationProof{}, skills.ErrPublicationUnconfirmed
	}
	return publicationProof{FileID: id, Revision: digestDocument(content, true)}, nil
}

func (p publication) place(ctx context.Context, proof publicationProof) ([]string, error) {
	// The original staged file identifies this publication even if a later edit
	// has changed its bytes. A same-content replacement cannot prove the move.
	for _, document := range []string{p.activeDocument(), filepath.Join(p.store.archiveDir(p.proposal.Name), skillspec.SkillFile)} {
		id, err := documentFileID(p.root, document)
		if err != nil {
			return nil, err
		}
		if id == proof.FileID {
			return nil, nil
		}
	}
	if err := p.verifyStage(proof); err != nil {
		return nil, err
	}
	if _, err := p.preimage(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, destination := p.stagedDirectory(), p.proposal.Name
	if p.proposal.Revises {
		source, destination = filepath.Join(source, skillspec.SkillFile), p.activeDocument()
	}
	if err := p.root.Rename(source, destination); err != nil {
		return nil, fmt.Errorf("skill publication: place %q: %w", p.proposal.Name, err)
	}
	return p.store.skillIdentities(p.proposal.Name), nil
}

func (p publication) verifyStage(proof publicationProof) error {
	directory := p.stagedDirectory()
	id, err := documentFileID(p.root, filepath.Join(directory, skillspec.SkillFile))
	if err != nil {
		return err
	}
	if id != proof.FileID {
		return skills.ErrPublicationUnconfirmed
	}
	content, found, err := readSkill(p.root, directory)
	if err != nil {
		return err
	}
	if digestDocument(content, found) != proof.Revision {
		return skills.ErrPublicationUnconfirmed
	}
	return nil
}
