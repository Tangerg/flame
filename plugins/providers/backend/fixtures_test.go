package main

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
)

func mustClientSpec(t testing.TB, provider Provider, key, endpoint string) clientSpec {
	t.Helper()
	spec, err := newClientSpec(modelprovider.Connection{Provider: string(provider), APIKey: key, BaseURL: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	return spec
}
