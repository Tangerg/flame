package webassets_test

import (
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/webassets"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCarrierPolicyIsAppliedOnlyToItsEntry(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"index.html": "<!doctype html>", "plugin-carrier.html": "<!doctype html>", "plugin-carrier-policy.txt": "(); webrtc=block; report-to=carrier\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := webassets.New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/", "/plugin-carrier.html"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", name, nil))
		actual := response.Header().Get("Connection-Allowlist")
		if name == "/plugin-carrier.html" && actual != "(); webrtc=block; report-to=carrier" {
			t.Fatalf("carrier policy: %q", actual)
		}
		if name == "/" && actual != "" {
			t.Fatalf("workbench acquired guest policy: %q", actual)
		}
	}
	if err := os.Remove(filepath.Join(root, "plugin-carrier-policy.txt")); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/plugin-carrier.html", nil))
	if response.Code != 503 {
		t.Fatalf("entry without its enforcement policy: %d", response.Code)
	}
}
