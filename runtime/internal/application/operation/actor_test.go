package operation

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestActorKeysKeepOriginalIdentityAcrossActivationAndRelease(t *testing.T) {
	first, err := NewActor("11111111-1111-4111-8111-111111111111", testsupport.Digest("release").String(), testsupport.Digest("authority").String(), "backend")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := NewActor(first.InstallationID().String(), testsupport.Digest("updated release").String(), testsupport.Digest("updated authority").String(), "replacement")
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewActor("22222222-2222-4222-8222-222222222222", first.Release().String(), first.Authority().String(), "backend")
	if err != nil {
		t.Fatal(err)
	}
	original := ScopedKey(WithActor(t.Context(), first), "original")
	if original != ScopedKey(WithActor(t.Context(), updated), "original") {
		t.Fatal("release replacement changed the logical command identity")
	}
	if original == ScopedKey(WithActor(t.Context(), other), "original") || original == ScopedKey(t.Context(), "original") {
		t.Fatal("another actor acquired the original command identity")
	}
	if ScopedKey(WithActor(t.Context(), first), "") != "" {
		t.Fatal("unkeyed command became keyed")
	}
	if err := (Actor{}).Validate(); err == nil {
		t.Fatal("zero actor was valid")
	}
}
