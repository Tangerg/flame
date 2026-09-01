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

	"github.com/Tangerg/oolong/core/program"
	"github.com/mattn/go-shellwords"
)

const maxExternalDraftBytes = 4 << 20

type promptEditor interface {
	Edit(context.Context, program.Session, string, string) (string, error)
}

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

func (d *draftEditor) Edit(ctx context.Context, session program.Session, workspace, original string) (string, error) {
	if d == nil || len(d.command) == 0 {
		return "", errors.New("external editor is unavailable")
	}
	temporary, err := os.CreateTemp("", "flame-prompt-*.md")
	if err != nil {
		return "", fmt.Errorf("create editor draft: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	if chmodErr := temporary.Chmod(0o600); chmodErr != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("protect editor draft: %w", chmodErr)
	}
	if _, writeStringErr := io.WriteString(temporary, original); writeStringErr != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write editor draft: %w", writeStringErr)
	}
	if syncErr := temporary.Sync(); syncErr != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync editor draft: %w", syncErr)
	}
	if closeErr := temporary.Close(); closeErr != nil {
		return "", fmt.Errorf("close editor draft: %w", closeErr)
	}
	arguments := append(slices.Clone(d.command[1:]), path)
	command := exec.CommandContext(ctx, d.command[0], arguments...) //nolint:gosec // The user explicitly configures their editor command.
	command.Dir = workspace
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if handErr := session.Hand(command.Run); handErr != nil {
		return "", fmt.Errorf("run external editor: %w", handErr)
	}
	source, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect edited draft: %w", err)
	}
	if err := validateEditedDraft(source); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open edited draft: %w", err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect edited draft: %w", err)
	}
	if !os.SameFile(source, opened) {
		return "", errors.New("edited draft changed while it was being opened")
	}
	if err := validateEditedDraft(opened); err != nil {
		return "", err
	}
	content, err := io.ReadAll(io.LimitReader(file, maxExternalDraftBytes+1))
	if err != nil {
		return "", fmt.Errorf("read edited draft: %w", err)
	}
	if len(content) > maxExternalDraftBytes {
		return "", fmt.Errorf("edited draft exceeds %d bytes", maxExternalDraftBytes)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return "", errors.New("edited draft is not valid text")
	}
	return strings.TrimRight(string(content), "\r\n"), nil
}

func validateEditedDraft(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("edited draft is not a regular file")
	}
	if info.Size() > maxExternalDraftBytes {
		return fmt.Errorf("edited draft exceeds %d bytes", maxExternalDraftBytes)
	}
	return nil
}

func (a *app) editPromptExternally() error {
	if a.execution.blocksAdmission() {
		return errors.New("finish or cancel the active run before opening an external editor")
	}
	message, err := a.composerMessage()
	if err != nil {
		return err
	}
	edited, err := a.editor.Edit(a.ctx, a.loop.Session(), a.session.current.Workspace.Path, message.Text)
	if err != nil {
		return err
	}
	message.Text = edited
	if err := a.recoverDraft(message); err != nil {
		return fmt.Errorf("prompt updated in external editor, but save session draft: %w", err)
	}
	a.message("updated prompt from external editor")
	return nil
}
