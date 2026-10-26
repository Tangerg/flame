package main

import (
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestManifestGoNamesRejectsCompetingProjections(t *testing.T) {
	for _, source := range []string{
		`{"methods":[]}`,
		`{"methods":[{"name":"a","goName":"Read"},{"name":"b","goName":"Read"}]}`,
		`{"methods":[{"name":"a","goName":"Read"},{"name":"a","goName":"Write"}]}`,
		`{"methods":[{"name":"","goName":"Read"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := manifestGoNames(path); err == nil {
			t.Fatalf("accepted competing method projections: %s", source)
		}
	}
}

func TestPublicAliasesRetainTheirPublicImportIdentity(t *testing.T) {
	private := types.NewPackage("example.com/dependency/internal/wire", "wire")
	id := types.NewNamed(types.NewTypeName(token.NoPos, private, "ID", nil), types.Typ[types.String], nil)
	public := types.NewPackage("example.com/dependency/protocol", "protocol")
	publicID := types.NewAlias(types.NewTypeName(token.NoPos, public, "ID", nil), id)
	owner := types.NewPackage("example.com/consumer", "consumer")
	name := types.NewTypeName(token.NoPos, owner, "ID", nil)
	types.NewAlias(name, publicID)
	owner.Scope().Insert(name)
	projected, err := projectPublicGoPackage(&packages.Package{PkgPath: owner.Path(), Types: owner})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(projected.Imports, []string{public.Path()}) || len(projected.Types) != 1 || projected.Types[0].Underlying != "protocol.ID" {
		t.Fatalf("public alias was flattened into dependency internals: %#v", projected)
	}
}
