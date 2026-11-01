package skillauthoring

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

const (
	publishedMarker = "published"
	refusedMarker   = "refused"
)

type publicationReceipt struct {
	root   *os.Root
	permit string
}

func (r publicationReceipt) mark(marker string) error {
	err := r.root.Mkdir(filepath.Join(r.permit, marker), 0700)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	return err
}
func (r publicationReceipt) has(marker string) (bool, error) {
	_, err := r.root.Stat(filepath.Join(r.permit, marker))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// The journal lives in host-private runtime storage. An atomic directory
// installation binds the permit to the original path, preimage and bytes.
func (r publicationReceipt) claim(p publication) (bool, error) {
	hash := sha256.New()
	for _, part := range []string{p.store.root, p.name, p.previous} {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	hash.Write(p.content)
	fingerprint := hex.EncodeToString(hash.Sum(nil))
	if found, err := r.has(fingerprint); found || err != nil {
		return false, err
	}
	if _, err := r.root.Stat(r.permit); err == nil {
		return false, skills.ErrProposalChanged
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	tmp := ".claim-" + rand.Text()
	if err := r.root.Mkdir(tmp, 0700); err != nil {
		return false, err
	}
	defer r.root.RemoveAll(tmp)
	if err := r.root.Mkdir(filepath.Join(tmp, fingerprint), 0700); err != nil {
		return false, err
	}
	if err := r.root.Rename(tmp, r.permit); err != nil {
		return false, err
	}
	return true, nil
}

func (r publicationReceipt) settle(publishErr error) error {
	if publishErr == nil {
		return r.mark(publishedMarker)
	}
	if errors.Is(publishErr, skills.ErrConflict) || errors.Is(publishErr, skills.ErrProposalChanged) || errors.Is(publishErr, skills.ErrLibraryCapacity) {
		return errors.Join(publishErr, r.mark(refusedMarker))
	}
	return publishErr
}
