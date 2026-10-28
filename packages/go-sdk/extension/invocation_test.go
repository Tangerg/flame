package extension_test

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
)

func TestOriginVocabularyBelongsToTheConsumer(t *testing.T) {
	invocation := portable.Invocation{
		ID: "document-convert", Origin: "document-workbench",
		Target: portable.Target{Scope: "document", Reference: "document-1"},
	}
	if err := invocation.Validate(); err != nil {
		t.Fatalf("independent consumer origin: %v", err)
	}
	if err := extension.ValidateInvocation(invocation); err == nil {
		t.Fatal("another consumer's origin was accepted as Flame evidence")
	}
	for _, origin := range []portable.Origin{extension.Human, extension.Model, extension.Automatic} {
		invocation.Origin = origin
		if err := extension.ValidateInvocation(invocation); err != nil {
			t.Fatalf("Flame origin %s: %v", origin, err)
		}
	}
	invocation.Parent = invocation.ID
	if err := extension.ValidateInvocation(invocation); err == nil {
		t.Fatal("invalid generic evidence bypassed consumer admission")
	}
}
