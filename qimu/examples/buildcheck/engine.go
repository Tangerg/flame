package buildcheck

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/qimu"
)

type Engine struct {
	Host      *qimu.Host
	Contracts Contracts
	Source    *Source
	scope     qimu.ScopeGrant
}

func New(ctx context.Context, source *Source) (*Engine, error) {
	if source == nil {
		return nil, errors.New("buildcheck: source is required")
	}
	contracts, err := NewContracts()
	if err != nil {
		return nil, err
	}
	host, err := qimu.NewHost(ctx, qimu.HostConfig{})
	if err != nil {
		return nil, err
	}
	engine := &Engine{Host: host, Contracts: contracts, Source: source}
	if err := host.Register(contracts.Passes, contracts.Analyzers, contracts.Formatters, contracts.Around); err != nil {
		return nil, err
	}
	plugins, err := StandardPlugins(contracts)
	if err != nil {
		return nil, err
	}
	ids := []qimu.InstallationID{"compiler", "analysis", "format", "trace"}
	var roots []qimu.ExportRef
	for i, plugin := range plugins {
		if _, err := host.Install(ctx, qimu.InstallationConfig{ID: ids[i], Plugin: plugin, Enabled: true}); err != nil {
			return nil, err
		}
		roots = append(roots, definitionExports(ids[i], plugin)...)
	}
	if _, err := host.Activate(ctx, roots...); err != nil {
		return nil, err
	}
	engine.scope, err = host.Admission().OpenScope(ctx, "build:source")
	if err != nil {
		return nil, err
	}
	return engine, nil
}

// Run reads the source once, then captures all four Points together. The
// framework protects implementations; Source remains the only writeback owner.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	source := e.Source.Read()
	snapshot, err := e.Host.Snapshot(ctx, e.Contracts.Passes.Ref(), e.Contracts.Analyzers.Ref(), e.Contracts.Formatters.Ref(), e.Contracts.Around.Ref())
	if err != nil {
		return Result{}, err
	}
	passes, err := qimu.Candidates(snapshot, e.Contracts.Passes)
	if err != nil {
		return Result{}, err
	}
	slices.SortFunc(passes, func(a, b qimu.Candidate[PassMetadata, CompilerPass]) int {
		return cmp.Compare(a.Metadata().Order, b.Metadata().Order)
	})
	analyzers, err := qimu.Candidates(snapshot, e.Contracts.Analyzers)
	if err != nil {
		return Result{}, err
	}
	slices.SortFunc(analyzers, func(a, b qimu.Candidate[AnalyzerMetadata, Analyzer]) int {
		return cmp.Compare(a.Metadata().Name, b.Metadata().Name)
	})
	formatters, err := qimu.Candidates(snapshot, e.Contracts.Formatters)
	if err != nil {
		return Result{}, err
	}
	slices.SortFunc(formatters, func(a, b qimu.Candidate[FormatterMetadata, Formatter]) int {
		return cmp.Or(cmp.Compare(b.Metadata().Priority, a.Metadata().Priority), cmp.Compare(a.Metadata().Name, b.Metadata().Name))
	})
	if len(formatters) == 0 {
		return Result{}, errors.New("buildcheck: no formatter is available")
	}
	around, err := qimu.Candidates(snapshot, e.Contracts.Around)
	if err != nil {
		return Result{}, err
	}
	slices.SortFunc(around, func(a, b qimu.Candidate[AroundMetadata, Around]) int {
		return cmp.Or(cmp.Compare(a.Metadata().Order, b.Metadata().Order), cmp.Compare(a.Metadata().Name, b.Metadata().Name))
	})
	passSelection, err := qimu.Select(snapshot, e.Contracts.Passes, "build.passes/1", passes...)
	if err != nil {
		return Result{}, err
	}
	analyzerSelection, err := qimu.Select(snapshot, e.Contracts.Analyzers, "build.analyzers/1", analyzers...)
	if err != nil {
		return Result{}, err
	}
	formatterSelection, err := qimu.Select(snapshot, e.Contracts.Formatters, "build.formatter/1", formatters[0])
	if err != nil {
		return Result{}, err
	}
	aroundSelection, err := qimu.Select(snapshot, e.Contracts.Around, "build.around/1", around...)
	if err != nil {
		return Result{}, err
	}
	plan, err := qimu.Capture(snapshot, passSelection.Ref(), analyzerSelection.Ref(), formatterSelection.Ref(), aroundSelection.Ref())
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = qimu.WithPlan(ctx, e.Host, plan, e.scope, func(use *qimu.Use) error {
		return qimu.Borrow(ctx, use, passSelection, func(ctx context.Context, passValues []CompilerPass) error {
			return qimu.Borrow(ctx, use, analyzerSelection, func(ctx context.Context, analyzerValues []Analyzer) error {
				return qimu.Borrow(ctx, use, formatterSelection, func(ctx context.Context, formatterValues []Formatter) error {
					return qimu.Borrow(ctx, use, aroundSelection, func(ctx context.Context, aroundValues []Around) error {
						orderedPasses := make([]namedPass, len(passes))
						for i, pass := range passes {
							orderedPasses[i] = namedPass{metadata: pass.Metadata(), call: passValues[i]}
						}
						parallelAnalyzers := make([]namedAnalyzer, len(analyzers))
						for i, analyzer := range analyzers {
							parallelAnalyzers[i] = namedAnalyzer{metadata: analyzer.Metadata(), call: analyzerValues[i]}
						}
						wrappers := make([]namedAround, len(around))
						for i, wrapper := range around {
							wrappers[i] = namedAround{metadata: wrapper.Metadata(), call: aroundValues[i]}
						}
						var err error
						result, err = execute(ctx, source, orderedPasses, parallelAnalyzers, formatterValues[0], wrappers)
						return err
					})
				})
			})
		})
	})
	return result, err
}

func (e *Engine) Close(ctx context.Context) error { _, err := e.Host.Close(ctx); return err }
