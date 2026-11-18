package main

import (
	"testing"
)

func openTestStore(t *testing.T, directory string) *store {
	t.Helper()
	store, err := openStore(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}
