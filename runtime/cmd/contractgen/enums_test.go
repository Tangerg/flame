package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"golang.org/x/tools/go/packages"
)

func enumFixture(t *testing.T, source string) *packages.Package {
	t.Helper()
	files := token.NewFileSet()
	syntax, err := parser.ParseFile(files, "vocabulary.go", "package vocabulary\n"+source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	checked, err := new(types.Config).Check("flame.invalid/vocabulary", files, []*ast.File{syntax}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &packages.Package{Fset: files, Types: checked, TypesInfo: info, Syntax: []*ast.File{syntax}}
}

func TestClosedEnumsUseTypedConstantsAndPreserveSourceOrder(t *testing.T) {
	pkg := enumFixture(t, `//flame:enum
type Status string
const ( First Status = "started"; Second = Status(last); last = "finished" )
type Open string
const Extension Open = "example"
`)
	got, err := closedEnums(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["Status"], []string{"started", "finished"}) {
		t.Fatalf("typed vocabulary = %v", got)
	}
	if _, exists := got["Open"]; exists {
		t.Fatal("unmarked extension vocabulary was closed")
	}
}

func TestInvalidClosedEnumMarkersAreRejected(t *testing.T) {
	for _, source := range []string{
		"//flame:enum\ntype Alias = string",
		"//flame:enum\ntype Number int\nconst One Number = 1",
		"//flame:enum\ntype Empty string",
		"//flame:enum\ntype (First string; Second string)",
	} {
		if _, err := closedEnums(enumFixture(t, source)); err == nil {
			t.Fatalf("invalid marker accepted: %s", source)
		}
	}
}

func TestDerivedWireVocabulariesRemainComplete(t *testing.T) {
	constants := enumConstants(protocolVocabulary())
	for _, typ := range []reflect.Type{reflect.TypeFor[protocol.RunProtocolFeature](), reflect.TypeFor[protocol.RuntimeEventType](), reflect.TypeFor[protocol.RuntimeTopic]()} {
		got, ok := enumValues(typ)
		if !ok || !slices.Equal(got, constants[typ.Name()]) {
			t.Errorf("%s owner = %v, constants = %v", typ.Name(), got, constants[typ.Name()])
		}
	}
}

func TestEnumValuesReturnsCallerOwnedValues(t *testing.T) {
	typ := reflect.TypeFor[protocol.RunStatus]()
	first, ok := enumValues(typ)
	if !ok || len(first) == 0 {
		t.Fatal("RunStatus has no closed vocabulary")
	}
	want := first[0]
	first[0] = "rewritten-by-caller"
	second, ok := enumValues(typ)
	if !ok || len(second) == 0 || second[0] != want {
		t.Fatal("caller changed the vocabulary owner")
	}
}
