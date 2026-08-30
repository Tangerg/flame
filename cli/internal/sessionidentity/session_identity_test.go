package sessionidentity

import (
	"strings"
	"testing"
)

func TestSessionIdentityIsExactAndOpaque(t *testing.T) {
	identity, err := Parse("ses_一/2")
	if err != nil {
		t.Fatal(err)
	}
	if identity.String() != "ses_一/2" {
		t.Fatalf("identity = %q", identity.String())
	}
	for _, value := range []string{
		"",
		" ses_1",
		"ses_1 ",
		"ses_ one",
		"ses_\n",
		"ses_\u200bhidden",
		"ses_\x00one",
		strings.Repeat("一", MaximumCharacters+1),
		string([]byte{0xff}),
	} {
		if _, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) accepted an invalid session identity", value)
		}
	}
	if err := (ID{}).Validate(); err == nil {
		t.Fatal("zero Session identity is valid")
	}
}
