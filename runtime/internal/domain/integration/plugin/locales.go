package plugin

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"golang.org/x/text/language"
)

const MaxLocaleTagBytes = 128
const maxLocales = 32

type Locale struct {
	Tag   string
	Title string
	Entry string
}

func (b *Builder) AdmitLocale(value Locale) error {
	if len(b.declaration.Locales) >= maxLocales {
		return refuse(DiagnosticComponentLimit, "locale capacity")
	}
	tag, err := language.Parse(value.Tag)
	base, _, _ := tag.Raw()
	if err != nil || tag.String() != value.Tag || base.String() == "und" || value.Tag == "en" || len(value.Tag) > MaxLocaleTagBytes {
		return refuse(DiagnosticInvalidDeclaration, "locale tag %q", value.Tag)
	}
	if slices.ContainsFunc(b.declaration.Locales, func(current Locale) bool { return current.Tag == value.Tag }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate locale %q", value.Tag)
	}
	if value.Title == "" || len(value.Title) > MaxPresentationTitleBytes || !utf8.ValidString(value.Title) {
		return refuse(DiagnosticInvalidDeclaration, "locale %q title", value.Tag)
	}
	if !ValidResourcePath(value.Entry) || !strings.HasPrefix(value.Entry, Namespace+"/") || !strings.HasSuffix(value.Entry, ".json") {
		return refuse(DiagnosticInvalidDeclaration, "locale %q entry", value.Tag)
	}
	b.declaration.Locales = append(b.declaration.Locales, value)
	return nil
}

func (i *Installation) AuthorizeLocale(release Release, digest identity.Digest, tag string) (Locale, error) {
	if err := i.authorizePresentation(release, digest); err != nil {
		return Locale{}, err
	}
	for _, value := range release.declaration.Locales {
		if value.Tag == tag {
			return value, nil
		}
	}
	return Locale{}, fmt.Errorf("%w: locale %q", ErrNotFound, tag)
}
