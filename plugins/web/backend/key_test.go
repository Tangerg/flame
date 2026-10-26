package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCredentialAbsenceIsDistinctFromInvalidConfigurationAndFormattingRedacts(t *testing.T) {
	unset, err := newAPIKey(nil)
	if err != nil || unset.configured() {
		t.Fatal("unset credential became configured")
	}
	for _, raw := range []string{"", " ", " key ", "key\n", string([]byte{0xff})} {
		if _, err := newAPIKey(&raw); err == nil {
			t.Fatal("invalid configured credential was accepted")
		}
	}
	secret := "private-web-fixture-key"
	key, err := newAPIKey(&secret)
	if err != nil || !key.configured() || key.value != secret {
		t.Fatal("configured credential changed")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if strings.Contains(fmt.Sprintf(format, key), secret) {
			t.Fatalf("format %s disclosed a credential", format)
		}
	}
}
