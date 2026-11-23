package localruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ProductRootVariable relocates the product root. Only an empty value selects
// the default beneath the user's home, so a configured root never needs one.
const ProductRootVariable = "FLAME_HOME"

const (
	productRootName  = ".flame"
	runtimeStateName = "runtime"
	databaseFilename = "flame-frame.db"
	localTokenName   = "local-token"
)

var ErrInvalidDataDirectory = errors.New("invalid local Runtime data directory")

type DataDirectory struct {
	path string
}

func DataDirectoryAt(path string) (DataDirectory, error) {
	if path == "" {
		return DataDirectory{}, invalidDataDirectory("path is required")
	}
	if !filepath.IsAbs(path) {
		return DataDirectory{}, invalidDataDirectory("path must be absolute")
	}
	return DataDirectory{path: filepath.Clean(path)}, nil
}

// The segment beneath the product root belongs to this package; a caller that
// appended its own would publish a second layout.
func DataDirectoryUnder(productRoot string) (DataDirectory, error) {
	if productRoot == "" {
		return DataDirectory{}, invalidDataDirectory("product root is required")
	}
	if !filepath.IsAbs(productRoot) {
		return DataDirectory{}, invalidDataDirectory("product root must be absolute")
	}
	return DataDirectoryAt(filepath.Join(filepath.Clean(productRoot), runtimeStateName))
}

func ProductRoot() (string, error) {
	configured := os.Getenv(ProductRootVariable)
	var userHome string
	if configured == "" {
		var err error
		userHome, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("local product root: locate user home: %w", err)
		}
	}
	return resolveProductRoot(userHome, configured)
}

func resolveProductRoot(userHome, configuredRoot string) (string, error) {
	if configuredRoot != "" {
		if !filepath.IsAbs(configuredRoot) {
			return "", fmt.Errorf("local product root: %s must be absolute", ProductRootVariable)
		}
		return filepath.Clean(configuredRoot), nil
	}
	if userHome == "" {
		return "", errors.New("local product root: user home is required")
	}
	if !filepath.IsAbs(userHome) {
		return "", errors.New("local product root: user home must be absolute")
	}
	return filepath.Join(userHome, productRootName), nil
}

func (d DataDirectory) Path() string { return d.path }

func (d DataDirectory) DatabasePath() string {
	return d.join(databaseFilename)
}

func (d DataDirectory) LocalTokenPath() string {
	return d.join(localTokenName)
}

func (d DataDirectory) join(name string) string {
	if d.path == "" {
		return ""
	}
	return filepath.Join(d.path, name)
}

func invalidDataDirectory(reason string) error {
	return fmt.Errorf("local Runtime data directory: %s: %w", reason, ErrInvalidDataDirectory)
}

func FirstPartyDirectory(executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("first-party distribution: executable must be absolute")
	}
	return filepath.Join(filepath.Dir(executable), "plugins"), nil
}

func RunnerExecutable(executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("runner distribution: executable must be absolute")
	}
	return filepath.Join(filepath.Dir(executable), "frame-runner"), nil
}
