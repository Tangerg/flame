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

	"github.com/spf13/fileflow"
	"github.com/spf13/pathologize"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
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

// Publication retains the selected path even when confirmation or staging
// cleanup fails after publication. Repeating the export is a separate intent.
type Publication struct {
	Path string
	filesystem.CommitResult
}

func (Store) Publish(workspace, title, requestedName string, document session.Document) (result Publication) {
	if err := document.Validate(); err != nil {
		result.Err = fmt.Errorf("publish session document: %w", err)
		return result
	}
	root, err := existingDirectory(workspace)
	if err != nil {
		result.Err = err
		return result
	}
	name, err := documentName(title, requestedName, document.Extension())
	if err != nil {
		result.Err = err
		return result
	}
	destination := pathologize.Join(root, name)
	staged, err := stage(root, document.Bytes())
	if err != nil {
		result.Err = err
		return result
	}
	defer func() {
		if err := os.Remove(staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Err = errors.Join(result.Err, fmt.Errorf("remove session staging file: %w", err))
		}
	}()
	flow := fileflow.Flow{FindAvailableName: fileflow.FindAvailableNameAuto, NoCreateDirs: true}
	// Staging is in the destination directory. Rename preserves the final path
	// on post-publication cleanup failure; Move discards that evidence.
	return publishStaged(func() (string, error) { return flow.Rename(staged, destination) }, confirmPublication)
}

func publishStaged(publish func() (string, error), confirm func(string) error) Publication {
	path, publishErr := publish()
	result := Publication{Path: path, CommitResult: filesystem.CommitResult{Err: publishErr}}
	if path == "" {
		return result
	}
	result.State = filesystem.CommitVisible
	if err := confirm(path); err != nil {
		result.Err = errors.Join(publishErr, fmt.Errorf("confirm session document durability: %w", err))
		return result
	}
	result.State = filesystem.CommitDurable
	return result
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
	file, info, err := filesystem.OpenFileExpected(path, source, int64(session.MaximumDocumentBytes))
	if err != nil {
		switch {
		case errors.Is(err, filesystem.ErrChanged):
			return session.Document{}, errors.New("session artifact changed while it was being opened")
		case errors.Is(err, filesystem.ErrNotRegular):
			return session.Document{}, errors.New("session artifact is not a regular file")
		case errors.Is(err, filesystem.ErrFileTooLarge):
			return session.Document{}, fmt.Errorf("session artifact exceeds %d bytes", session.MaximumDocumentBytes)
		default:
			return session.Document{}, fmt.Errorf("open session artifact: %w", err)
		}
	}
	defer func() { _ = file.Close() }()
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
	if err := filesystem.VerifyPathVersion(file, info, path); err != nil {
		if errors.Is(err, filesystem.ErrChanged) {
			return session.Document{}, errors.New("session artifact changed while it was being read")
		}
		return session.Document{}, fmt.Errorf("verify session artifact after reading: %w", err)
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
	root, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(path)
	if err != nil {
		return "", fmt.Errorf("resolve session document workspace: %w", err)
	}
	return root, nil
}

func resolveInputPath(workspace, selected string) (string, error) {
	if strings.TrimSpace(selected) == "" {
		return "", errors.New("import path is empty")
	}
	root := ""
	if !filepath.IsAbs(selected) {
		var err error
		root, err = existingDirectory(workspace)
		if err != nil {
			return "", err
		}
	}
	resolved, err := filesystem.ResolvePath(root, selected)
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
		if closeErr := temporary.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close session document staging file: %w", closeErr))
		}
		if err != nil {
			if removeErr := os.Remove(temporary.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove session document staging file: %w", removeErr))
			}
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

func confirmPublication(path string) error {
	file, info, err := filesystem.OpenFile(path, int64(session.MaximumDocumentBytes))
	if err != nil {
		return err
	}
	// An identical existing destination may have been selected instead of the
	// staged inode, so confirm the actual published bytes as well as its name.
	if err := errors.Join(file.Sync(), filesystem.VerifyPathVersion(file, info, path), file.Close()); err != nil {
		return err
	}
	directory, _, err := filesystem.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
