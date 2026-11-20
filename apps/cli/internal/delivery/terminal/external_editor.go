package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/mattn/go-shellwords"
)

type draftEditor struct {
	command []string
}

func configuredDraftEditor() (*draftEditor, error) {
	configured := ""
	for _, name := range []string{"FLAME_EDITOR", "VISUAL", "EDITOR"} {
		if configured = strings.TrimSpace(os.Getenv(name)); configured != "" {
			break
		}
	}
	if configured == "" {
		configured = "vi"
	}
	command, err := shellwords.Parse(configured)
	if err != nil {
		return nil, fmt.Errorf("parse editor command: %w", err)
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, errors.New("editor command is empty")
	}
	return &draftEditor{command: command}, nil
}

func (d *draftEditor) Edit(ctx context.Context, session program.Session, workspace, original string) (edited string, err error) {
	if len(original) > authoring.MaxMessageTextBytes {
		return "", fmt.Errorf("editor draft exceeds %d bytes", authoring.MaxMessageTextBytes)
	}
	path, err := writeEditorDraft(original)
	if err != nil {
		return "", err
	}
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			edited = ""
			err = errors.Join(err, fmt.Errorf("remove editor draft: %w", removeErr))
		}
	}()
	arguments := append(slices.Clone(d.command[1:]), path)
	command := exec.CommandContext(ctx, d.command[0], arguments...) //nolint:gosec // The user explicitly configures their editor command.
	command.Dir = workspace
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if handErr := session.Hand(command.Run); handErr != nil {
		return "", fmt.Errorf("run external editor: %w", handErr)
	}
	return readEditedDraft(path)
}

// writeEditorDraft stores original in a private temporary file and returns its
// path. A failed write leaves no file behind.
func writeEditorDraft(original string) (_ string, err error) {
	temporary, err := os.CreateTemp("", "flame-prompt-*.md")
	if err != nil {
		return "", fmt.Errorf("create editor draft: %w", err)
	}
	path := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("protect editor draft: %w", err)
	}
	if _, err := io.WriteString(temporary, original); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write editor draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync editor draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close editor draft: %w", err)
	}
	return path, nil
}

// readEditedDraft reads back the file the editor saved, refusing one that was
// replaced, grew past the message limit or changed while it was read.
func readEditedDraft(path string) (string, error) {
	source, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect edited draft: %w", err)
	}
	if err := validateEditedDraft(source); err != nil {
		return "", err
	}
	file, opened, err := filesystem.OpenFileExpected(path, source, authoring.MaxMessageTextBytes)
	if err != nil {
		return "", openedDraftError(err)
	}
	defer func() { _ = file.Close() }()
	if err := validateEditedDraft(opened); err != nil {
		return "", err
	}
	content, err := io.ReadAll(io.LimitReader(file, authoring.MaxMessageTextBytes+1))
	if err != nil {
		return "", fmt.Errorf("read edited draft: %w", err)
	}
	if len(content) > authoring.MaxMessageTextBytes {
		return "", fmt.Errorf("edited draft exceeds %d bytes", authoring.MaxMessageTextBytes)
	}
	if err := filesystem.VerifyPathVersion(file, opened, path); err != nil {
		if errors.Is(err, filesystem.ErrChanged) {
			return "", errors.New("edited draft changed while it was being read")
		}
		return "", fmt.Errorf("verify edited draft after reading: %w", err)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return "", errors.New("edited draft is not valid text")
	}
	return strings.TrimRight(string(content), "\r\n"), nil
}

func openedDraftError(err error) error {
	switch {
	case errors.Is(err, filesystem.ErrChanged):
		return errors.New("edited draft changed while it was being opened")
	case errors.Is(err, filesystem.ErrNotRegular):
		return errors.New("edited draft is not a regular file")
	case errors.Is(err, filesystem.ErrFileTooLarge):
		return fmt.Errorf("edited draft exceeds %d bytes", authoring.MaxMessageTextBytes)
	default:
		return fmt.Errorf("open edited draft: %w", err)
	}
}

func validateEditedDraft(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("edited draft is not a regular file")
	}
	if info.Size() > authoring.MaxMessageTextBytes {
		return fmt.Errorf("edited draft exceeds %d bytes", authoring.MaxMessageTextBytes)
	}
	return nil
}

// editExternally replaces the prompt text with what the configured editor
// saves, keeping the prompt's attachments.
func (c *promptComposer) editExternally() error {
	if c.busy() {
		return errors.New("finish or cancel the active run before opening an external editor")
	}
	message, err := c.message()
	if err != nil {
		return err
	}
	edited, err := c.editor.Edit(c.ctx, c.terminal, authoringDirectory(c.localDirectory, c.session.current.Workspace.Path), message.Text)
	if err != nil {
		return err
	}
	message.Text = edited
	if err := c.recoverDraft(message); err != nil {
		return fmt.Errorf("prompt updated in external editor, but save session draft: %w", err)
	}
	c.notify("updated prompt from external editor")
	return nil
}
