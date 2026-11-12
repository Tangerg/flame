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

func TestOAuthCallbackRejectsMalformedResponsesWithoutConsumingAttempt(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
	}{
		{"malformed issuer encoding", "code=authorization-code&state=exact-state&iss=%ZZ"},
		{"malformed extension encoding", "code=authorization-code&state=exact-state&extension=%ZZ"},
		{"invalid field name UTF-8", "code=authorization-code&state=exact-state&%FF=value"},
		{"invalid code UTF-8", "code=%FF&state=exact-state"},
		{"invalid error UTF-8", "error=%FF&state=exact-state"},
		{"invalid issuer UTF-8", "code=authorization-code&state=exact-state&iss=%FF"},
		{"invalid extension UTF-8", "code=authorization-code&state=exact-state&extension=%FF"},
		{"unescaped separator", "code=authorization-code&state=exact-state&iss=https://issuer.example;ignored=value"},
		{"conflicting issuers", "code=authorization-code&state=exact-state&iss=https://issuer.example&iss=https://unexpected.example"},
		{"repeated issuer", "code=authorization-code&state=exact-state&iss=https://issuer.example&iss=https://issuer.example"},
		{"repeated code", "code=authorization-code&code=another-code&state=exact-state"},
		{"repeated state", "code=authorization-code&state=exact-state&state=exact-state"},
		{"repeated error", "error=access_denied&error=server_error&state=exact-state"},
	} {
		t.Run(test.name, func(t *testing.T) {
			flow := &oauthFlow{result: make(chan oauthCallback, 1)}
			result, err := flow.waitForCallback(t.Context(), "https://authorization.example?state=exact-state", func(context.Context, string) error {
				rejected := httptest.NewRecorder()
				flow.handleCallback(rejected, httptest.NewRequest(http.MethodGet, "/callback?"+test.query, nil))
				if rejected.Code != http.StatusBadRequest {
					t.Fatalf("malformed callback status = %d, want %d", rejected.Code, http.StatusBadRequest)
				}
				accepted := httptest.NewRecorder()
				flow.handleCallback(accepted, httptest.NewRequest(http.MethodGet, "/callback?code=authorization-code&state=exact-state&iss=https://issuer.example&extension=%E4%B8%AD", nil))
				if accepted.Code != http.StatusOK {
					t.Fatalf("valid callback status = %d, want %d", accepted.Code, http.StatusOK)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Code != "authorization-code" || result.State != "exact-state" || result.Iss != "https://issuer.example" {
				t.Fatalf("valid authorization evidence changed: %+v", result)
			}
		})
	}
}
