// Package project locates the project a working directory belongs to, and the
// directories between the two.
package project

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// marker is the entry whose presence makes a directory a project root. A
// submodule records it as a file rather than a directory, so only its existence
// is asked about.
const marker = ".git"

// Root returns the nearest ancestor of cwd holding the marker, or cwd when no
// ancestor does. cwd is cleaned once, so an equivalent path yields an
// equivalent root.
//
// A directory that cannot be inspected is an error rather than an absence.
// Reading a permission or I/O failure as "no project here" walks past the real
// root and answers with a different one, and the root is the key a hook-trust
// decision and a document cascade are looked up by.
func Root(cwd string) (string, error) {
	current := filepath.Clean(cwd)
	for {
		switch _, err := os.Stat(filepath.Join(current, marker)); {
		case err == nil:
			return current, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd), nil
		}
		current = parent
	}
}

// Chain returns [root, ..., cwd], inclusive at both ends, so a caller folding
// the most specific value last can range over it in order. A cwd equal to root
// yields one element.
func Chain(cwd, root string) []string {
	if cwd == root {
		return []string{cwd}
	}
	var chain []string
	current := cwd
	for current != root {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	chain = append(chain, root)
	slices.Reverse(chain)
	return chain
}
