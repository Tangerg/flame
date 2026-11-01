package model

import (
	"errors"
	"testing"
)

func TestEndpointPolicyKeepsGatewayMountSeparateFromSDKVersion(t *testing.T) {
	mount := EndpointPolicy{Example: "https://example.invalid/mount", AppendsPath: "/v1"}
	if err := mount.Validate(); err != nil {
		t.Fatal(err)
	}
	if mount.EffectivePath("") != "" || mount.EffectivePath("https://example.invalid/v1") != "/v1/v1" {
		t.Fatal("incorrect route projection")
	}
	if err := mount.ValidateOverride("https://example.invalid/tenant/v1"); err != nil {
		t.Fatal(err)
	}
	suffix := EndpointPolicy{Example: "https://example.invalid/openai/v1", RequiredSuffix: "/openai/v1"}
	if err := suffix.ValidateOverride("https://example.invalid/tenant/openai/v1"); err != nil {
		t.Fatal(err)
	}
	if err := suffix.ValidateOverride("https://example.invalid/v1"); !errors.Is(err, ErrEndpointOverride) {
		t.Fatal(err)
	}
	if err := (EndpointPolicy{Example: mount.Example, AppendsPath: "/v1", RequiredSuffix: "/v1"}).Validate(); err == nil {
		t.Fatal("accepted competing path rules")
	}
}
