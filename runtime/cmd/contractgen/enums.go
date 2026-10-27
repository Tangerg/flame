package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"golang.org/x/tools/go/packages"
)

var protocolVocabulary = sync.OnceValue(func() *packages.Package {
	path := reflect.TypeFor[protocol.RunStatus]().PkgPath()
	loaded, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo}, path)
	if err != nil {
		panic(fmt.Errorf("load protocol vocabulary: %w", err))
	}
	if len(loaded) != 1 {
		panic(fmt.Errorf("load protocol vocabulary: expected one package, got %d", len(loaded)))
	}
	if len(loaded[0].Errors) != 0 {
		panic(fmt.Errorf("load protocol vocabulary: %v", loaded[0].Errors))
	}
	return loaded[0]
})

var closedVocabulary = sync.OnceValue(func() map[string][]string {
	values, err := closedEnums(protocolVocabulary())
	if err != nil {
		panic(err)
	}
	return values
})

func enumValues(t reflect.Type) ([]string, bool) {
	// An external protocol vocabulary publishes its own closed values. Reuse
	// that declaration instead of introducing Flame-specific enum annotations.
	if values, ok := reflect.Zero(t).Interface().(interface{ Values() []string }); ok {
		return slices.Clone(values.Values()), true
	}
	switch t {
	case reflect.TypeFor[protocol.RunProtocolFeature]():
		return runProtocolFeatureValues(), true
	case reflect.TypeFor[protocol.RuntimeEventType]():
		return runtimeEventValues(), true
	case reflect.TypeFor[protocol.RuntimeTopic]():
		return runtimeTopicValues(), true
	}
	if t.PkgPath() != reflect.TypeFor[protocol.RunStatus]().PkgPath() {
		return nil, false
	}
	values, ok := closedVocabulary()[t.Name()]
	return slices.Clone(values), ok
}

// A marker closes the type; Go's type checker owns values and conversions.
func closedEnums(pkg *packages.Package) (map[string][]string, error) {
	marked := map[string]bool{}
	for _, file := range pkg.Syntax {
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			if enumMarker(group.Doc) && len(group.Specs) != 1 {
				return nil, fmt.Errorf("enum marker requires one named type")
			}
			for _, item := range group.Specs {
				spec := item.(*ast.TypeSpec)
				if !enumMarker(spec.Doc) && !enumMarker(group.Doc) {
					continue
				}
				object, ok := pkg.TypesInfo.Defs[spec.Name].(*types.TypeName)
				if !ok || object.IsAlias() {
					return nil, fmt.Errorf("enum %s must be a named string type", spec.Name.Name)
				}
				basic, ok := object.Type().Underlying().(*types.Basic)
				if !ok || basic.Kind() != types.String {
					return nil, fmt.Errorf("enum %s must have string underlying type", object.Name())
				}
				marked[object.Name()] = true
			}
		}
	}
	values := enumConstants(pkg)
	result := make(map[string][]string, len(marked))
	for name := range marked {
		if len(values[name]) == 0 {
			return nil, fmt.Errorf("enum %s has no typed constants", name)
		}
		result[name] = values[name]
	}
	return result, nil
}

func enumMarker(group *ast.CommentGroup) bool {
	if group == nil {
		return false
	}
	for _, comment := range group.List {
		if strings.TrimSpace(comment.Text) == "//flame:enum" {
			return true
		}
	}
	return false
}

func enumConstants(pkg *packages.Package) map[string][]string {
	files := slices.Clone(pkg.Syntax)
	slices.SortFunc(files, func(a, b *ast.File) int {
		return strings.Compare(pkg.Fset.Position(a.Pos()).Filename, pkg.Fset.Position(b.Pos()).Filename)
	})
	values := map[string][]string{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, item := range group.Specs {
				for _, name := range item.(*ast.ValueSpec).Names {
					value, ok := pkg.TypesInfo.Defs[name].(*types.Const)
					if !ok || value.Val().Kind() != constant.String {
						continue
					}
					named, ok := types.Unalias(value.Type()).(*types.Named)
					if !ok || named.Obj().Pkg() != pkg.Types {
						continue
					}
					key := named.Obj().Name()
					text := constant.StringVal(value.Val())
					if !slices.Contains(values[key], text) {
						values[key] = append(values[key], text)
					}
				}
			}
		}
	}
	return values
}

func runProtocolFeatureValues() []string {
	var values []string
	for _, feature := range protocol.Features() {
		if feature.RequiredByRunProtocol {
			values = append(values, feature.Key)
		}
	}
	return values
}

// runtimeEventValues is the event vocabulary: every topic, plus resync. Derived from
// the topic list rather than written twice — that duplication is exactly what having
// one set of strings prevents.
func runtimeEventValues() []string {
	out := make([]string, 0, len(protocol.RuntimeTopics())+1)
	out = append(out, runtimeTopicValues()...)
	return append(out, string(protocol.RuntimeResync))
}

func runtimeTopicValues() []string {
	topics := protocol.RuntimeTopics()
	out := make([]string, 0, len(topics))
	for _, topic := range topics {
		out = append(out, string(topic))
	}
	return out
}
