package buildcheck

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/qimu"
)

func StandardPlugins(contracts Contracts) ([]*qimu.Plugin, error) {
	definitions := []qimu.Definition{
		{Revision: qimu.Hash([]byte("buildcheck/compiler/1")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "compiler", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "normalize", Contract: contracts.Passes.Ref()}, {Name: "emit", Contract: contracts.Passes.Ref()}}}, Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{
				qimu.Contribute("normalize", contracts.Passes, PassMetadata{Stage: "normalize", Order: 10}, CompilerPass(func(ctx context.Context, input string) (string, error) {
					if err := ctx.Err(); err != nil {
						return "", err
					}
					return strings.TrimSpace(input), nil
				})),
				qimu.Contribute("emit", contracts.Passes, PassMetadata{Stage: "emit", Order: 20}, CompilerPass(func(ctx context.Context, input string) (string, error) {
					if err := ctx.Err(); err != nil {
						return "", err
					}
					return "compiled(" + input + ")", nil
				})),
			}, nil
		}}}},
		{Revision: qimu.Hash([]byte("buildcheck/analysis/1")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "analysis", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "todo", Contract: contracts.Analyzers.Ref()}, {Name: "long-line", Contract: contracts.Analyzers.Ref()}}}, Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{
				qimu.Contribute("todo", contracts.Analyzers, AnalyzerMetadata{Name: "todo", Required: true}, Analyzer(func(ctx context.Context, input SourceRevision) ([]Diagnostic, error) {
					var found []Diagnostic
					for line, text := range strings.Split(input.Text, "\n") {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
						if strings.Contains(text, "TODO") {
							found = append(found, Diagnostic{Analyzer: "todo", Line: line + 1, Message: "unfinished work"})
						}
					}
					return found, nil
				})),
				qimu.Contribute("long-line", contracts.Analyzers, AnalyzerMetadata{Name: "long-line"}, Analyzer(func(ctx context.Context, input SourceRevision) ([]Diagnostic, error) {
					var found []Diagnostic
					for line, text := range strings.Split(input.Text, "\n") {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
						if len(text) > 80 {
							found = append(found, Diagnostic{Analyzer: "long-line", Line: line + 1, Message: "line exceeds 80 bytes"})
						}
					}
					return found, nil
				})),
			}, nil
		}}}},
		{Revision: qimu.Hash([]byte("buildcheck/format/1")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "format", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "trim", Contract: contracts.Formatters.Ref()}, {Name: "identity", Contract: contracts.Formatters.Ref()}}}, Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{
				qimu.Contribute("trim", contracts.Formatters, FormatterMetadata{Name: "trim", Priority: 10}, Formatter(func(ctx context.Context, input SourceRevision) (Patch, error) {
					if err := ctx.Err(); err != nil {
						return Patch{}, err
					}
					return Patch{ExpectedRevision: input.Revision, Text: strings.TrimSpace(input.Text) + "\n"}, nil
				})),
				qimu.Contribute("identity", contracts.Formatters, FormatterMetadata{Name: "identity", Priority: 1}, Formatter(func(ctx context.Context, input SourceRevision) (Patch, error) {
					if err := ctx.Err(); err != nil {
						return Patch{}, err
					}
					return Patch{ExpectedRevision: input.Revision, Text: input.Text}, nil
				})),
			}, nil
		}}}},
		{Revision: qimu.Hash([]byte("buildcheck/trace/1")), Units: []qimu.Unit{{Declaration: qimu.UnitDeclaration{ID: "trace", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "trace", Contract: contracts.Around.Ref()}}}, Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{
				qimu.Contribute("trace", contracts.Around, AroundMetadata{Name: "trace", Order: 1}, Around(func(ctx context.Context, input SourceRevision, next Next) (Result, error) {
					if next == nil {
						return Result{}, errors.New("buildcheck: continuation is required")
					}
					result, err := next(ctx, input)
					result.Trace = append([]string{"trace:enter"}, result.Trace...)
					result.Trace = append(result.Trace, "trace:leave")
					return result, err
				})),
			}, nil
		}}}},
	}
	plugins := make([]*qimu.Plugin, 0, len(definitions))
	for _, definition := range definitions {
		plugin, err := qimu.Define(definition)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}
