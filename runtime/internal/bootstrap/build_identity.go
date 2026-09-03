package bootstrap

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

// ExecutableBuildID returns the content identity of the running executable.
func ExecutableBuildID() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("bootstrap: resolve executable for build identity: %w", err)
	}
	return buildIDFromFile(path)
}

func buildIDFromFile(path string) (string, error) {
	file, before, err := fileinput.Open(path, 0)
	if err != nil {
		return "", fmt.Errorf("bootstrap: open executable for build identity: %w", err)
	}
	defer func() { _ = file.Close() }()
	return buildIDFromOpenedFile(path, file, before)
}

func buildIDFromOpenedFile(path string, file *os.File, before os.FileInfo) (string, error) {
	if file == nil || before == nil || before.Size() < 0 || before.Size() == math.MaxInt64 {
		return "", errors.New("bootstrap: executable metadata cannot be hashed safely")
	}
	hash := sha256.New()
	written, err := io.CopyN(hash, file, before.Size()+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("bootstrap: hash executable for build identity: %w", err)
	}
	if written != before.Size() {
		return "", errors.New("bootstrap: executable size changed while hashing build identity")
	}
	after, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("bootstrap: inspect hashed executable: %w", err)
	}
	current, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("bootstrap: inspect executable after hashing build identity: %w", err)
	}
	if !sameBuildFileVersion(before, after) || !sameBuildFileVersion(after, current) {
		return "", errors.New("bootstrap: executable changed while hashing build identity")
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return runtimeidentity.BuildFromSHA256(digest).String(), nil
}

func sameBuildFileVersion(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) &&
		left.Size() == right.Size() && left.Mode() == right.Mode() && left.ModTime().Equal(right.ModTime())
}
