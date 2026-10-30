package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type localePackages struct {
	installationPackages
	reads    int
	expected plugin.Locale
	failure  error
}

func (p *localePackages) ReadLocale(ctx context.Context, installation *plugin.Installation, locale plugin.Locale) (map[string]string, error) {
	p.reads++
	if locale != p.expected || installation.Selected() == (identity.Digest{}) {
		return nil, errors.New("wrong locale release")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]string{"common.loading": "読込中"}, p.failure
}

func TestLocaleReadAdmitsExactEnabledDeclarationBeforeContentAccess(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	locale := plugin.Locale{Tag: "ja", Title: "日本語", Entry: plugin.Namespace + "/ja.json"}
	release := testsupport.Release(t, "locale", plugin.Declaration{Name: "locales", Locales: []plugin.Locale{locale}})
	installation, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	catalog[release.Digest()] = release
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	content := &localePackages{expected: locale}
	coordinator, err := New(t.Context(), testFramework(t, store, dependencies), store, catalog, content, installationConnections{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Locale(t.Context(), id, release.Digest(), "ja"); !errors.Is(err, plugin.ErrDisabled) || content.reads != 0 {
		t.Fatalf("disabled read=%v, reads=%d", err, content.reads)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		digest identity.Digest
		tag    string
		want   error
	}{
		{testsupport.Digest("other"), "ja", plugin.ErrStale},
		{release.Digest(), "ko", plugin.ErrNotFound},
	} {
		if _, err := coordinator.Locale(t.Context(), id, test.digest, test.tag); !errors.Is(err, test.want) || content.reads != 0 {
			t.Fatalf("rejected read=%v, reads=%d", err, content.reads)
		}
	}
	messages, err := coordinator.Locale(t.Context(), id, release.Digest(), "ja")
	if err != nil || messages["common.loading"] != "読込中" || content.reads != 1 {
		t.Fatalf("catalog=%v, %v", messages, err)
	}
	failure := errors.New("content store failed")
	content.failure = failure
	if _, err := coordinator.Locale(t.Context(), id, release.Digest(), "ja"); !errors.Is(err, failure) {
		t.Fatalf("read failure lost: %v", err)
	}
}
