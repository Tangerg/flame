package settings

import "testing"

func TestRemoteAuthoringFollowsCanonicalRouting(t *testing.T) {
	partition := func(s string) string {
		t.Helper()
		value, err := (Runtime{Mode: RuntimeRemote, Endpoint: s}).AuthoringPartition()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if partition("https://EXAMPLE.test/a/") != partition("https://example.test:443/a") {
		t.Fatal("same route split authoring ownership")
	}
	if partition("https://example.test/a") == partition("https://example.test/b") {
		t.Fatal("different proxy prefixes share a partition")
	}
}
