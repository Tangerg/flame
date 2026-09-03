package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/program"
)

func TestConfiguredDraftEditorParsesArgumentsWithoutExecutingShellSyntax(t *testing.T) {
	t.Setenv("FLAME_EDITOR", `code --wait "profile name"`)
	t.Setenv("VISUAL", "ignored")
	editor, err := configuredDraftEditor()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--wait", "profile name"}
	if strings.Join(editor.command, "|") != strings.Join(want, "|") {
		t.Fatalf("editor command = %#v", editor.command)
	}
}

func TestDraftEditorReadsSuccessfulEditsAndReportsFailure(t *testing.T) {
	workspace := t.TempDir()
	editor := &draftEditor{command: []string{"sh", "-c", `printf '\nrevised' >> "$0"`}}
	edited, err := editor.Edit(t.Context(), program.Session{}, workspace, "original")
	if err != nil {
		t.Fatal(err)
	}
	if edited != "original\nrevised" {
		t.Fatalf("edited draft = %q", edited)
	}

	failing := &draftEditor{command: []string{"sh", "-c", "exit 7"}}
	if _, err := failing.Edit(t.Context(), program.Session{}, workspace, "preserve me"); err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("failing editor error = %v", err)
	}
}

func TestDraftEditorHonorsApplicationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	editor := &draftEditor{command: []string{"sh", "-c", "sleep 30"}}
	if _, err := editor.Edit(ctx, program.Session{}, t.TempDir(), "preserve me"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled editor error = %v, want context.Canceled", err)
	}
}

func TestDraftEditorRejectsInvalidReplacementFiles(t *testing.T) {
	workspace := t.TempDir()
	text := filepath.Join(workspace, "text.txt")
	if err := os.WriteFile(text, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(workspace, "binary")
	if err := os.WriteFile(binary, []byte{'x', 0, 'y'}, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{
			name: "symbolic link",
			command: []string{
				"sh", "-c", `rm "$1" && ln -s "$0" "$1"`, text,
			},
			want: "not a regular file",
		},
		{
			name: "NUL byte",
			command: []string{
				"sh", "-c", `cp "$0" "$1"`, binary,
			},
			want: "not valid text",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&draftEditor{command: test.command}).Edit(t.Context(), program.Session{}, workspace, "original")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Edit error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDraftEditorReportsCleanupFailure(t *testing.T) {
	workspace := t.TempDir()
	record := filepath.Join(workspace, "draft-path")
	editor := &draftEditor{command: []string{
		"sh", "-c", `printf '%s' "$1" > "$0" && rm "$1" && mkdir "$1" && printf 'retained' > "$1/child"`, record,
	}}
	_, err := editor.Edit(t.Context(), program.Session{}, workspace, "sensitive prompt")
	draftPath, readErr := os.ReadFile(record)
	if readErr != nil {
		t.Fatal(readErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(string(draftPath)) })
	if err == nil {
		t.Fatal("Edit succeeded despite retaining its temporary draft path")
	}
	for _, want := range []string{"edited draft is not a regular file", "remove editor draft"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Edit error = %v, want %q", err, want)
		}
	}
}
