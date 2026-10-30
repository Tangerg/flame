package pluginpackage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestLocalePackageAdmitsOnlyBoundedFlatStringCatalogs(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"partial", `{"common.loading":"読込中","literal.key":"<text> {{count}}"}`, true},
		{"empty", `{}`, false},
		{"array", `[]`, false},
		{"empty key", `{"":"value"}`, false},
		{"null", `null`, false},
		{"null value", `{"key":null}`, false},
		{"number", `{"key":1}`, false},
		{"nested", `{"key":{"nested":"value"}}`, false},
		{"duplicate", `{"key":"first","key":"second"}`, false},
		{"invalid unicode", `{"key":"\ud800"}`, false},
		{"too large", `{"key":"` + strings.Repeat("a", int(maxManifestBytes)) + `"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			releases := testReleases(t)
			release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{
				"plugin.json":                          `{"$schema":"` + manifestSchema + `","name":"test.locales","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				plugin.Namespace + "/declaration.json": flameDeclaration(`"contributes":{"locales":[{"tag":"ja","title":"日本語","entry":"io.github.tangerg.flame/ja.json"}]}`),
				plugin.Namespace + "/ja.json":          test.body,
			}))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if !test.valid {
				if len(declaration.Locales) != 0 || len(declaration.Diagnostics) != 1 {
					t.Fatalf("invalid catalog admitted: %+v", declaration)
				}
				return
			}
			if len(declaration.Locales) != 1 || len(declaration.Servers) != 0 || len(declaration.Components) != 0 || len(declaration.Diagnostics) != 0 {
				t.Fatalf("content-only package: %+v", declaration)
			}
			installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := releases.ReadLocale(t.Context(), installation, declaration.Locales[0])
			if err != nil || messages["common.loading"] != "読込中" || messages["literal.key"] != "<text> {{count}}" {
				t.Fatalf("catalog=%v, %v", messages, err)
			}
			canceled, cancel := context.WithCancel(t.Context())
			cancel()
			if messages, err := releases.ReadLocale(canceled, installation, declaration.Locales[0]); len(messages) != 0 || !errors.Is(err, context.Canceled) || errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("canceled catalog=%v, %v", messages, err)
			}
		})
	}
}
