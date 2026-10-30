package secrets

import (
	json "encoding/json/v2"
	"testing"
	"unicode/utf8"
)

func TestMask(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", want: ""},
		{name: "short", value: "abc123", want: "****"},
		{name: "boundary", value: "12345678", want: "****"},
		{name: "long", value: "sk-ant-0000fc78", want: "sk****78"},
		{name: "short Chinese", value: "你好世界", want: "****"},
		{name: "short emoji", value: "🔑🔐🔒🔓", want: "****"},
		{name: "short combining sequences", value: "e\u0301e\u0301e\u0301e\u0301", want: "****"},
		{name: "long Unicode", value: "甲乙丙丁戊己庚辛壬癸", want: "甲乙****壬癸"},
		{name: "mixed Unicode", value: "🔑a1234567b🔐", want: "🔑a****b🔐"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Mask(test.value)
			if got != test.want {
				t.Fatalf("Mask(%q) = %q, want %q", test.value, got, test.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("mask is not valid UTF-8: %x", got)
			}
			if _, err := json.Marshal(map[string]string{"masked": got}); err != nil {
				t.Fatalf("mask is not JSON representable: %v", err)
			}
			if test.value != "" && got == test.value {
				t.Fatal("mask disclosed the complete secret")
			}
		})
	}
}
