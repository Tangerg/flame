package plugin

import (
	"errors"
	"strings"
	"testing"
)

func TestLocaleAdmissionRequiresCanonicalLanguageAndLocalJSON(t *testing.T) {
	for _, test := range []struct {
		name  string
		value Locale
		valid bool
	}{
		{"language", Locale{Tag: "ja", Title: "日本語", Entry: Namespace + "/ja.json"}, true},
		{"regional", Locale{Tag: "zh-TW", Title: "繁體中文", Entry: Namespace + "/zh-TW.json"}, true},
		{"noncanonical", Locale{Tag: "zh-tw", Title: "Chinese", Entry: Namespace + "/zh.json"}, false},
		{"reserved", Locale{Tag: "en", Title: "Replacement", Entry: Namespace + "/en.json"}, false},
		{"undetermined", Locale{Tag: "und", Title: "Unknown", Entry: Namespace + "/und.json"}, false},
		{"private only", Locale{Tag: "x-private", Title: "Private", Entry: Namespace + "/x.json"}, false},
		{"invalid", Locale{Tag: "../ja", Title: "Japanese", Entry: Namespace + "/ja.json"}, false},
		{"empty title", Locale{Tag: "ja", Entry: Namespace + "/ja.json"}, false},
		{"long title", Locale{Tag: "ja", Title: strings.Repeat("a", 129), Entry: Namespace + "/ja.json"}, false},
		{"outside namespace", Locale{Tag: "ja", Title: "Japanese", Entry: "other/ja.json"}, false},
		{"traversal", Locale{Tag: "ja", Title: "Japanese", Entry: Namespace + "/../ja.json"}, false},
		{"code", Locale{Tag: "ja", Title: "Japanese", Entry: Namespace + "/ja.js"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder, err := NewBuilder("locales", "", "")
			if err != nil {
				t.Fatal(err)
			}
			err = builder.AdmitLocale(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("admission=%v, valid=%v", err, test.valid)
			}
			if test.valid && builder.AdmitLocale(test.value) == nil {
				t.Fatal("duplicate language admitted")
			}
		})
	}
}

func TestLocaleAuthorizationFollowsSelectedEnabledRelease(t *testing.T) {
	locale := Locale{Tag: "ja", Title: "Japanese", Entry: Namespace + "/ja.json"}
	release := testRelease(t, "locale", Declaration{Name: "locales", Locales: []Locale{locale}})
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installation.AuthorizeLocale(release, release.Digest(), "ja"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled read=%v", err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if value, err := installation.AuthorizeLocale(release, release.Digest(), "ja"); err != nil || value != locale {
		t.Fatalf("authorized locale=%+v, %v", value, err)
	}
	declaration := release.Declaration()
	declaration.Locales[0].Entry = Namespace + "/replacement.json"
	if value, _ := installation.AuthorizeLocale(release, release.Digest(), "ja"); value.Entry != locale.Entry {
		t.Fatal("projection changed admitted catalog")
	}
	if _, err := installation.AuthorizeLocale(release, release.Digest(), "ko"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undeclared read=%v", err)
	}
	replacement := testRelease(t, "replacement", release.Declaration())
	if err := installation.Stage(release, replacement); err != nil {
		t.Fatal(err)
	}
	if err := installation.Select(release, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := installation.AuthorizeLocale(replacement, release.Digest(), "ja"); !errors.Is(err, ErrStale) {
		t.Fatalf("replaced read=%v", err)
	}
}
