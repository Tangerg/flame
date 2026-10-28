package distribution

import (
	"context"
	"errors"
	"os"

	"github.com/Tangerg/flame/qimu"
)

// VerifyArchive validates a publisher artifact in disposable storage. It makes
// no publisher trust decision and never installs code into an application Host.
func VerifyArchive(ctx context.Context, archive string, digest qimu.Digest, limits Limits) (_ Release, err error) {
	directory, err := os.MkdirTemp("", "qimu-verify-")
	if err != nil {
		return Release{}, err
	}
	defer func() { err = errors.Join(err, removeSealed(directory)) }()
	store, err := Open(Config{
		Directory: directory, Admission: qimu.NewAdmission(), Limits: limits,
		Trust:    func(context.Context, Source, Manifest, Lock) error { return nil },
		Retained: func(context.Context, qimu.Digest) (bool, error) { return true, nil },
	})
	if err != nil {
		return Release{}, err
	}
	defer func() { err = errors.Join(err, store.Close(context.Background())) }()
	return store.InstallArchive(ctx, archive, digest)
}
