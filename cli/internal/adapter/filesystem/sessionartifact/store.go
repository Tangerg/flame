// Package sessionartifact safely loads and publishes portable session files.
package sessionartifact

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/spf13/fileflow"
	"github.com/spf13/pathologize"

	"github.com/Tangerg/flame/cli/internal/application/agent/session"
)

const (
	portableFilenameByteLimit = 255
	// fileflow tries conflict suffixes through -100. Reserve the largest one
	// so every name it derives remains portable as well as the first choice.
	maximumConflictSuffixBytes = len("-100")
)

// Store owns the filesystem boundary for session documents. Its zero value is
// ready to use and publishes without overwriting different existing content.
type Store struct{}

func (Store) Publish(workspace, title, requestedName string, document session.Document) (string, error) {
	if err := document.Validate(); err != nil {
		return "", fmt.Errorf("publish session document: %w", err)
	}
	root, err := existingDirectory(workspace)
	if err != nil {
		return "", err
	}
	name, err := documentName(title, requestedName, document.Extension())
	if err != nil {
		return "", err
	}
	destination := pathologize.Join(root, name)
	staged, err := stage(root, document.Bytes())
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(staged) }()
	flow := fileflow.Flow{FindAvailableName: fileflow.FindAvailableNameAuto, NoCreateDirs: true}
	finalPath, err := flow.Move(staged, destination)
	if err != nil {
		return "", fmt.Errorf("publish session document: %w", err)
	}
	// Move has already published the selected final path. Directory sync is a
	// crash-durability strengthening step, not a reason to report a false failure
	// that would make a retry publish another conflict-renamed document.
	syncCommittedDirectory(root)
	return finalPath, nil
}

// Load reads an explicitly selected JSON artifact. Relative paths resolve from
// the active workspace; absolute paths remain valid so users can move sessions
// between projects without first copying them into the destination workspace.
func (Store) Load(workspace, selectedPath string) (session.Document, error) {
	path, err := resolveInputPath(workspace, selectedPath)
	if err != nil {
		return session.Document{}, err
	}
	source, err := os.Lstat(path)
	if err != nil {
		return session.Document{}, fmt.Errorf("inspect session artifact: %w", err)
	}
	if err := validateDocumentSource(source); err != nil {
		return session.Document{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return session.Document{}, fmt.Errorf("open session artifact: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return session.Document{}, fmt.Errorf("inspect session artifact: %w", err)
	}
	if !os.SameFile(source, info) {
		return session.Document{}, errors.New("session artifact changed while it was being opened")
	}
	if err := validateDocumentSource(info); err != nil {
		return session.Document{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, session.MaximumDocumentBytes+1))
	if err != nil {
		return session.Document{}, fmt.Errorf("read session artifact: %w", err)
	}
	if len(body) > session.MaximumDocumentBytes {
		return session.Document{}, fmt.Errorf("session artifact exceeds %d bytes", session.MaximumDocumentBytes)
	}
	document, err := session.NewDocument(protocol.ExportFormatJSON, body)
	if err != nil {
		return session.Document{}, fmt.Errorf("read session artifact: %w", err)
	}
	return document, nil
}

func validateDocumentSource(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("session artifact is not a regular file")
	}
	if info.Size() > int64(session.MaximumDocumentBytes) {
		return fmt.Errorf("session artifact exceeds %d bytes", session.MaximumDocumentBytes)
	}
	return nil
}

func existingDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("session document workspace is empty")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve session document workspace: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve session document workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect session document workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("session document workspace is not a directory")
	}
	return root, nil
}

func resolveInputPath(workspace, selected string) (string, error) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", errors.New("import path is empty")
	}
	if !filepath.IsAbs(selected) {
		root, err := existingDirectory(workspace)
		if err != nil {
			return "", err
		}
		selected = filepath.Join(root, selected)
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("resolve session artifact: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve session artifact: %w", err)
	}
	return resolved, nil
}

func documentName(title, requested, desiredExtension string) (string, error) {
	requested = strings.TrimSpace(requested)
	var stem, extension string
	if requested == "" {
		stem = strings.TrimSpace(title)
		if stem == "" {
			stem = "flame-session"
		}
		extension = desiredExtension
	} else {
		if strings.ContainsAny(requested, `/\`) || filepath.Base(requested) != requested || requested == "." || requested == ".." {
			return "", errors.New("export name must be a filename, not a path")
		}
		extension = filepath.Ext(requested)
		if extension == "" {
			extension = desiredExtension
			stem = requested
		} else {
			if !strings.EqualFold(extension, desiredExtension) {
				return "", fmt.Errorf("export filename must end in %s", desiredExtension)
			}
			stem = strings.TrimSuffix(requested, extension)
		}
	}
	maximumStemBytes := portableFilenameByteLimit - maximumConflictSuffixBytes - len(extension)
	if maximumStemBytes <= 0 {
		return "", errors.New("export filename extension leaves no room for a name")
	}
	stem = pathologize.Clean(stem)
	if stem == "_" {
		stem = "flame-session"
	}
	stem = truncateUTF8(stem, maximumStemBytes)
	cleaned := stem + extension
	if !pathologize.IsClean(cleaned) {
		return "", errors.New("export filename is not portable after normalization")
	}
	return cleaned, nil
}

func truncateUTF8(value string, maximumBytes int) string {
	if len(value) <= maximumBytes {
		return value
	}
	end := maximumBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func stage(root string, body []byte) (path string, err error) {
	temporary, err := os.CreateTemp(root, ".flame-session-*")
	if err != nil {
		return "", fmt.Errorf("create session document staging file: %w", err)
	}
	path = temporary.Name()
	defer func() {
		if closeErr := temporary.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close session document staging file: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = temporary.Write(body); err != nil {
		return "", fmt.Errorf("write session document staging file: %w", err)
	}
	if err = temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync session document staging file: %w", err)
	}
	return path, nil
}

func syncCommittedDirectory(path string) {
	directory, err := os.Open(path)
	if err != nil {
		return
	}
	_ = errors.Join(directory.Sync(), directory.Close())
}
