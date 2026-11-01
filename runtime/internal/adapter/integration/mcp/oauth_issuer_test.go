package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthCallbackPreservesIssuerForSDKValidation(t *testing.T) {
	for _, issuer := range []string{"", "https://issuer.example", "https://unexpected.example"} {
		t.Run(issuer, func(t *testing.T) {
			flow := &oauthFlow{result: make(chan oauthCallback, 1)}
			query := url.Values{"code": {"authorization-code"}, "state": {"exact-state"}, "iss": {issuer}}
			request := httptest.NewRequest("GET", "/callback?"+query.Encode(), nil)
			response := httptest.NewRecorder()
			result, err := flow.waitForCallback(t.Context(), "https://authorization.example?state=exact-state", func(context.Context, string) error {
				for _, noise := range []string{"", "?state=wrong&code=bad", "?state=wrong&error=denied"} {
					rejected := httptest.NewRecorder()
					flow.handleCallback(rejected, httptest.NewRequest(http.MethodGet, "/callback"+noise, nil))
					if rejected.Code != http.StatusBadRequest {
						t.Fatalf("unrelated callback status = %d", rejected.Code)
					}
				}
				flow.handleCallback(response, request)
				return nil
			})
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
