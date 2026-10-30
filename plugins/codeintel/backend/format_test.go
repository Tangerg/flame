package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
)

func TestFormattingTransformsCapturedContentWithoutWritingTheWorkspace(t *testing.T) {
	for _, test := range []struct{ path, content, want string }{
		{"main.go", "package main\nfunc main(){println(\"ok\")}\n", "package main\n\nfunc main() { println(\"ok\") }\n"},
		{"data.json", `{"b":1,"a":2}`, "{\n  \"b\": 1,\n  \"a\": 2\n}\n"},
	} {
		t.Run(test.path, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, test.path)
			if err := os.WriteFile(path, []byte("different on disk"), 0600); err != nil {
				t.Fatal(err)
			}
			source, err := codeintel.NewDocument(root, test.path, test.content)
			if err != nil {
				t.Fatal(err)
			}
			result, err := formatDocument(t.Context(), source)
			if err != nil || result.Content == nil || *result.Content != test.want || result.Version != source.Version {
				t.Fatalf("transformation: %+v %v", result, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "different on disk" {
				t.Fatalf("workspace changed: %q %v", data, err)
			}
		})
	}
}

func TestRunFormatterBoundsDiagnosticOutput(t *testing.T) {
	_, err := runFormatter(
		t.Context(),
		nil,
		"/bin/sh",
		"-c",
		"/usr/bin/yes x | /usr/bin/head -c 131072 >&2; exit 1",
		"fixture.ts",
	)
	if err == nil {
		t.Fatal("runFormatter error = nil, want formatter failure")
	}
	if len(err.Error()) > (64<<10)+1024 {
		t.Fatalf("formatter diagnostic uses %d bytes, want bounded material", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("formatter error = %q, want honest truncation marker", err)
	}
}

func TestRunFormatterPreservesCancellation(t *testing.T) {
	cause := errors.New("formatting retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	_, err := runFormatter(ctx, nil, "gofmt", "-w", filepath.Join(t.TempDir(), "file.go"))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("formatter error = %v, want cancellation category and owner cause", err)
	}
}
