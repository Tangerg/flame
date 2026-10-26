package mcp

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/go-sdk/auth"
)

func TestOAuthCallbackPreservesIssuerForSDKValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the browser-launch fixture uses a POSIX shell")
	}
	bin := t.TempDir()
	launcher := "xdg-open"
	if runtime.GOOS == "darwin" {
		launcher = "open"
	}
	if err := os.WriteFile(filepath.Join(bin, launcher), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, issuer := range []string{"", "https://issuer.example", "https://unexpected.example"} {
		t.Run(issuer, func(t *testing.T) {
			flow := &oauthFlow{result: make(chan oauthCallback, 1)}
			query := url.Values{"code": {"authorization-code"}, "state": {"exact-state"}, "iss": {issuer}}
			request := httptest.NewRequest("GET", "/callback?"+query.Encode(), nil)
			response := httptest.NewRecorder()
			flow.handleCallback(response, request)
			result, err := flow.fetch(t.Context(), &auth.AuthorizationArgs{URL: "https://authorization.example"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Code != "authorization-code" || result.State != "exact-state" || result.Iss != issuer {
				t.Fatalf("fetch lost authorization evidence: %+v", result)
			}
			if strings.Contains(response.Body.String(), "Authorized —") {
				t.Fatal("callback announced authorization before SDK validation and token exchange")
			}
		})
	}
}
