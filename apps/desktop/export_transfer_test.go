package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

type selectedExportDestination string

func (p selectedExportDestination) ChooseFileDestination(string) (string, error) {
	return string(p), nil
}

func TestExportPublishesCompleteFilesUnderOneTransfer(t *testing.T) {
	for _, test := range []struct{ name, content string }{
		{name: "empty"},
		{name: "nonempty", content: "new export"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const filename = "export.txt"
			const oldContent = "previous export"
			directory := t.TempDir()
			destination := filepath.Join(directory, filename)
			if err := os.WriteFile(destination, []byte(oldContent), 0o600); err != nil {
				t.Fatal(err)
			}
			host := DesktopHost{shell: &nativeShell{exports: selectedExportDestination(destination)}}
			t.Cleanup(func() {
				if err := host.transfer.abort(); err != nil {
					t.Error(err)
				}
			})
			id, err := host.BeginFileSave(filename, len(test.content))
			if err != nil || id == "" {
				t.Fatalf("begin export = %q, %v", id, err)
			}
			if _, err := host.BeginFileSave(filename, 1); err == nil {
				t.Fatal("admitted another export during the first transfer")
			}
			if test.content != "" {
				encoded := base64.StdEncoding.EncodeToString([]byte(test.content))
				if err := host.AppendFileSave(id, 0, encoded); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(destination)
			if err != nil || string(before) != oldContent {
				t.Fatalf("destination before commit = %q, %v", before, err)
			}
			if err := host.FinishFileSave(id); err != nil {
				t.Fatal(err)
			}
			published, err := os.ReadFile(destination)
			if err != nil || string(published) != test.content {
				t.Fatalf("published export = %q, %v, want %q", published, err, test.content)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("export directory = %v, %v, want only the published file", entries, err)
			}
		})
	}
}
