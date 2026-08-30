package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/Tangerg/flame/runtime/internal/buildidentity"
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
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("bootstrap: open executable for build identity: %w", err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("bootstrap: hash executable for build identity: %w", err)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return buildidentity.FromSHA256(digest).String(), nil
}
