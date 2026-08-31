package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestCredentialFormattingNeverRevealsTheAPIKey(t *testing.T) {
	const secret = "sk-format-must-not-escape"
	key, err := NewAPIKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := New("openai")
	if err != nil {
		t.Fatal(err)
	}
	entry, err = entry.Apply(Patch{APIKey: Set(key)})
	if err != nil {
		t.Fatal(err)
	}
	credential, _ := entry.Credential()

	for _, value := range []any{key, credential, entry, Set(key), Patch{APIKey: Set(key)}} {
		for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
			if rendered := fmt.Sprintf(format, value); strings.Contains(rendered, secret) {
				t.Fatalf("format %q of %T revealed credential material", format, value)
			}
		}
	}
}

func TestProviderCredentialIsClosedAndStoredWinsEnvironmentFallback(t *testing.T) {
	storedKey, err := NewAPIKey("sk-stored")
	if err != nil {
		t.Fatal(err)
	}
	environmentKey, err := NewAPIKey("sk-environment")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := New("openai")
	if err != nil {
		t.Fatal(err)
	}
	entry, err = entry.Apply(Patch{APIKey: Set(storedKey)})
	if err != nil {
		t.Fatal(err)
	}
	entry = entry.WithEnvironmentFallback(environmentKey)

	credential, configured := entry.Credential()
	if !configured {
		t.Fatal("stored provider should be configured")
	}
	key, _ := credential.APIKey()
	source, _ := credential.Source()
	if key.Reveal() != "sk-stored" || source != KeyStored {
		t.Fatalf("credential = (%q, %q), want stored credential", key.Reveal(), source)
	}

	unconfigured, err := New("anthropic")
	if err != nil {
		t.Fatal(err)
	}
	unconfigured = unconfigured.WithEnvironmentFallback(environmentKey)
	credential, configured = unconfigured.Credential()
	source, _ = credential.Source()
	if !configured || source != KeyEnvironment {
		t.Fatalf("environment credential = (%v, %q)", configured, source)
	}
}

func TestPatchDistinguishesPreserveSetAndClearWithoutStringSentinels(t *testing.T) {
	key, _ := NewAPIKey("sk-old")
	oldBaseURL, _ := NewBaseURL("https://old.test")
	newBaseURL, _ := NewBaseURL("https://new.test/")
	entry, _ := New("openai")
	entry, _ = entry.Apply(Patch{APIKey: Set(key), BaseURL: Set(oldBaseURL)})

	updated, err := entry.Apply(Patch{BaseURL: Set(newBaseURL)})
	if err != nil {
		t.Fatal(err)
	}
	updatedKey, _ := updated.APIKey()
	baseURL, _ := updated.BaseURL()
	if updatedKey.Reveal() != key.Reveal() || baseURL.String() != "https://new.test" {
		t.Fatalf("replace endpoint = (%q, %q)", updatedKey.Reveal(), baseURL.String())
	}

	updated, err = updated.Apply(Patch{APIKey: Clear[APIKey]()})
	if err != nil {
		t.Fatal(err)
	}
	if _, configured := updated.Credential(); configured {
		t.Fatal("cleared provider should be unconfigured")
	}
	baseURL, present := updated.BaseURL()
	if !present || baseURL.String() != "https://new.test" {
		t.Fatalf("clearing key changed endpoint = (%q, %v)", baseURL.String(), present)
	}
	clearing := Patch{APIKey: Clear[APIKey]()}
	if !(Patch{}).Empty() || clearing.Empty() {
		t.Fatal("Patch.Empty does not distinguish preserve from clear")
	}
}

func TestProviderValuesRejectInvalidPrimitiveStates(t *testing.T) {
	if _, err := New("  "); !errors.Is(err, ErrIDRequired) {
		t.Fatalf("New blank id error = %v", err)
	}
	for _, value := range []string{
		"open ai",
		"openai\x00shadow",
		strings.Repeat("p", modelref.MaximumProviderIdentityCharacters+1),
	} {
		if _, err := New(value); !errors.Is(err, ErrIDInvalid) {
			t.Errorf("New(%q) error = %v, want ErrIDInvalid", value, err)
		}
	}
	if _, err := NewAPIKey("\t"); !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("NewAPIKey blank error = %v", err)
	}
	for _, value := range []string{
		"", "api.example.test", "ftp://api.example.test", "https://user@api.example.test",
		"https://api.example.test?token=secret", "https://api.example.test#fragment",
		"https://api.example.test/%zz",
	} {
		if _, err := NewBaseURL(value); !errors.Is(err, ErrBaseURLInvalid) {
			t.Errorf("NewBaseURL(%q) error = %v", value, err)
		}
	}
	entry, _ := New("openai")
	if _, err := entry.Apply(Patch{APIKey: Set(APIKey{})}); !errors.Is(err, ErrChangeCorrupted) {
		t.Fatalf("Apply zero API key error = %v", err)
	}
}
