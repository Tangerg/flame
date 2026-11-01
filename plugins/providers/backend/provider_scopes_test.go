package main

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"os"
	"slices"
	"testing"
)

var writeProviderScopes = flag.Bool("write-provider-scopes", false, "regenerate the packaged model adapter scope resource")

func TestPublishedProviderScopes(t *testing.T) {
	profiles := providers.published()
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	slices.Sort(ids)
	expected, err := json.Marshal(ids, jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	expected = append(expected, '\n')
	const resource = "../package/io.github.tangerg.flame/model-providers.generated.json"
	if *writeProviderScopes {
		if err := os.WriteFile(resource, expected, 0644); err != nil {
			t.Fatal(err)
		}
	}
	published, err := os.ReadFile(resource)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, published) {
		t.Fatal("model adapter scope resource is stale; run go generate ./plugins/providers/backend")
	}
}
