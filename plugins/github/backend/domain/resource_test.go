package resource

import (
	"fmt"
	"strings"
	"testing"
)

func TestResourceValuesConfineRequestsToRepositoryPaths(t *testing.T) {
	repository, err := NewRepository("Owner/Repo.Name")
	if err != nil || repository.String() != "owner/repo.name" || repository.APIPath() != "/repos/owner/repo.name" {
		t.Fatalf("repository = %v, %v", repository, err)
	}
	for _, input := range []string{"", "owner", "../repo", "owner/..", "owner/repo/other", "https://example.test/repo", "owner/repo?token=x", "owner/%2e%2e"} {
		if _, err := NewRepository(input); err == nil {
			t.Fatalf("unconfined repository admitted: %q", input)
		}
	}
	for _, input := range []string{"", "/etc/passwd", "../secret", "dir/../secret", "dir//file", `dir\file`, "file\x00name"} {
		if _, err := NewFilePath(input); err == nil {
			t.Fatalf("unconfined path admitted: %q", input)
		}
	}
	path, err := NewFilePath("docs/read me#%2F.md")
	if err != nil || path.APIPath() != "docs/read%20me%23%252F.md" {
		t.Fatalf("escaped path = %v, %v", path, err)
	}
}

func TestCredentialHasOneOptionalPresenceAndRedactsEveryFormattingVerb(t *testing.T) {
	public, err := NewCredential(nil)
	if err != nil || public.Configured() || public.Authorization() != "" {
		t.Fatal("absent credential did not select public access")
	}
	for _, value := range []string{"", " padded", "tab\tvalue", "line\nvalue", "non-ascii-界"} {
		if _, err := NewCredential(&value); err == nil {
			t.Fatal("invalid configured credential silently selected public access")
		}
	}
	token := "private-fixture-token"
	credential, err := NewCredential(&token)
	if err != nil {
		t.Fatal(err)
	}
	if !credential.Configured() || credential.Authorization() != "Bearer "+token {
		t.Fatal("credential did not authorize its explicit consumer")
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if strings.Contains(fmt.Sprintf(verb, credential), token) {
			t.Fatal("formatting disclosed the credential")
		}
	}
	if credential.CacheKey("resource") == public.CacheKey("resource") || credential.CacheKey("resource") == credential.CacheKey("another") {
		t.Fatal("conditional response identity lost credential or resource scope")
	}
}
