package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

func workspacePathspec(dir, path string) (string, error) {
	if path == "" {
		return ":(literal).", nil
	}
	relative := filepath.Clean(path)
	if filepath.IsAbs(path) {
		var err error
		if relative, err = filepath.Rel(dir, path); err != nil {
			return "", fmt.Errorf("git: resolve workspace-relative path: %w", err)
		}
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git: path %q is outside workspace %q", path, dir)
	}
	return ":(literal)" + filepath.ToSlash(relative), nil
}
