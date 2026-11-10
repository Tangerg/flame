package skilldocument

import (
	"errors"
	"io"
	"io/fs"
	"testing"
)

func TestLoadPreservesFencesInsideYAMLAndMarkdown(t *testing.T) {
	const instructions = "\n    printf 'first'\n\tprintf 'second'\nline with hard break  \nnext line\n---\n\n"
	content := []byte("---\nname: formatting\ndescription: |\n  Before\n  ---\n  After\n---\n" + instructions)
	skill, err := Load(t.Context(), "formatting", content)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Instructions != instructions {
		t.Fatalf("instructions = %q, want %q", skill.Instructions, instructions)
	}
}

func TestDocumentResourcesStayWithinTheirCapturedDirectory(t *testing.T) {
	files := documentFS{name: "captured", content: []byte("original content")}
	for _, test := range []struct{ dir, name string }{
		{"captured", "../captured/SKILL.md"},
		{"captured/..", "captured/SKILL.md"},
		{"/captured", "SKILL.md"},
		{"captured", "/SKILL.md"},
	} {
		if file, err := files.OpenInDir(test.dir, test.name); file != nil || !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("open (%q, %q) = %v, %v", test.dir, test.name, file, err)
		}
	}
	if file, err := files.OpenInDir("other", "SKILL.md"); file != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("another directory = %v, %v", file, err)
	}
	file, err := files.OpenInDir("captured", "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "original content" {
		t.Fatalf("captured resource = %q, %v", content, err)
	}
}
