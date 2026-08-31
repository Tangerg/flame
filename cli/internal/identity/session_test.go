package identity

import (
	"strings"
	"testing"
)

func TestSessionIdentityIsExactAndOpaque(t *testing.T) {
	if err := ValidateSession("ses_一/2"); err != nil {
		t.Fatalf("valid session identity: %v", err)
	}
	for _, value := range []string{
		"",
		" ses_1",
		"ses_1 ",
		"ses_ one",
		"ses_\n",
		"ses_\u200bhidden",
		"ses_\x00one",
		strings.Repeat("一", MaximumResourceCharacters+1),
		string([]byte{0xff}),
	} {
		if err := ValidateSession(value); err == nil {
			t.Errorf("ValidateSession(%q) accepted an invalid session identity", value)
		}
	}
}
