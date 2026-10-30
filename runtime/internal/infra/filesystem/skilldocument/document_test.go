package skilldocument

import "testing"

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
