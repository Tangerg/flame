package native

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/examples/document"
)

const Installation qimu.InstallationID = "authored-document"

type Workbench struct {
	Host      *qimu.Host
	Contracts document.Contracts
	scope     qimu.ScopeGrant
	output    string
}

func Open(ctx context.Context, config Configuration, outputDirectory string) (_ *Workbench, err error) {
	if outputDirectory == "" {
		return nil, errors.New("authoring: output directory is required")
	}
	contracts, err := document.NewContracts()
	if err != nil {
		return nil, err
	}
	host, err := qimu.NewHost(ctx, qimu.HostConfig{})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_, closeErr := host.Close(context.WithoutCancel(ctx))
			err = errors.Join(err, closeErr)
		}
	}()
	if err := host.Register(contracts.Files, contracts.Converters, contracts.Validators); err != nil {
		return nil, err
	}
	plugin, err := Plugin(contracts)
	if err != nil {
		return nil, err
	}
	configuration, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	installed, err := host.Install(ctx, qimu.InstallationConfig{ID: Installation, Plugin: plugin, Configuration: configuration, Enabled: true})
	if err != nil {
		return nil, err
	}
	var roots []qimu.ExportRef
	for _, unit := range installed.Units {
		for _, export := range unit.Exports {
			roots = append(roots, qimu.ExportRef{InstallationID: installed.ID, Release: installed.Release, UnitID: unit.ID, Export: export.Name, Contract: export.Contract})
		}
	}
	if _, err := host.Activate(ctx, roots...); err != nil {
		return nil, err
	}
	scope, err := host.Admission().OpenScope(ctx, "authoring:documents")
	if err != nil {
		return nil, err
	}
	return &Workbench{Host: host, Contracts: contracts, scope: scope, output: outputDirectory}, nil
}

func (w *Workbench) Close(ctx context.Context) error {
	_, err := w.Host.Close(ctx)
	return err
}

type Prepared struct {
	host       *qimu.Host
	scope      qimu.ScopeGrant
	plan       *qimu.BindingPlan
	converter  qimu.Selection[document.Converter]
	validators qimu.Selection[document.Validator]
	output     string
}

// Prepare applies consumer policy to one snapshot of both Points. Acquisition
// later validates that same complete read set; it does not select again.
func (w *Workbench) Prepare(ctx context.Context) (*Prepared, error) {
	snapshot, err := w.Host.Snapshot(ctx, w.Contracts.Converters.Ref(), w.Contracts.Validators.Ref())
	if err != nil {
		return nil, err
	}
	converter, err := qimu.SelectOne(snapshot, w.Contracts.Converters, "authoring.text-priority/1", func(values []qimu.Candidate[document.ConverterMetadata, document.Converter]) (qimu.Candidate[document.ConverterMetadata, document.Converter], error) {
		values = slices.DeleteFunc(values, func(value qimu.Candidate[document.ConverterMetadata, document.Converter]) bool {
			return value.Metadata().Format != "text"
		})
		slices.SortFunc(values, func(a, b qimu.Candidate[document.ConverterMetadata, document.Converter]) int {
			return cmp.Or(cmp.Compare(b.Metadata().Priority, a.Metadata().Priority), cmp.Compare(a.ID(), b.ID()))
		})
		if len(values) == 0 {
			return qimu.Candidate[document.ConverterMetadata, document.Converter]{}, errors.New("authoring: no text converter is available")
		}
		return values[0], nil
	})
	if err != nil {
		return nil, err
	}
	validators, err := qimu.SelectMany(snapshot, w.Contracts.Validators, "authoring.validator-order/1", func(values []qimu.Candidate[document.ValidatorMetadata, document.Validator]) ([]qimu.Candidate[document.ValidatorMetadata, document.Validator], error) {
		slices.SortFunc(values, func(a, b qimu.Candidate[document.ValidatorMetadata, document.Validator]) int {
			return cmp.Or(cmp.Compare(a.Metadata().Order, b.Metadata().Order), cmp.Compare(a.Metadata().Name, b.Metadata().Name), cmp.Compare(a.ID(), b.ID()))
		})
		return values, nil
	})
	if err != nil {
		return nil, err
	}
	plan, err := qimu.Capture(snapshot, converter.Ref(), validators.Ref())
	if err != nil {
		return nil, err
	}
	return &Prepared{host: w.Host, scope: w.scope, plan: plan, converter: converter, validators: validators, output: w.output}, nil
}

type Report struct {
	Converted   document.Converted
	Diagnostics []document.Diagnostic
}

// View is an acquired resource result. Its real temporary file and all selected
// providers remain in the original Use until Close or caller-lifetime closure.
type View struct {
	use    *qimu.Use
	file   *os.File
	size   int64
	report Report
}

func (p *Prepared) Acquire(ctx context.Context, owner *qimu.Lifetime, reference document.DocumentRef) (_ *View, err error) {
	use, err := p.host.Acquire(ctx, p.plan, qimu.UseConfig{Owner: owner, Scope: p.scope})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_, closeErr := use.Close(context.WithoutCancel(ctx))
			err = errors.Join(err, closeErr)
		}
	}()
	var report Report
	if err := qimu.Borrow(ctx, use, p.converter, func(ctx context.Context, values []document.Converter) error {
		var err error
		report.Converted, err = values[0](ctx, reference)
		return err
	}); err != nil {
		return nil, err
	}
	if err := qimu.Borrow(ctx, use, p.validators, func(ctx context.Context, values []document.Validator) error {
		for _, validate := range values {
			diagnostics, err := validate(ctx, report.Converted)
			if err != nil {
				return err
			}
			report.Diagnostics = append(report.Diagnostics, diagnostics...)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	work, err := use.Lifetime().Enter(ctx)
	if err != nil {
		return nil, err
	}
	defer work.Finish()
	file, err := os.CreateTemp(p.output, "view-*.html")
	if err != nil {
		return nil, err
	}
	release := func() error { return errors.Join(file.Close(), os.Remove(file.Name())) }
	if _, err := work.Cleanup("authored document view", release); err != nil {
		return nil, errors.Join(err, release())
	}
	written, err := io.WriteString(file, report.Converted.HTML)
	if err != nil {
		return nil, err
	}
	if err := context.Cause(work.Context()); err != nil {
		return nil, err
	}
	return &View{use: use, file: file, size: int64(written), report: report}, nil
}

func (v *View) Report() Report {
	result := v.report
	result.Diagnostics = slices.Clone(result.Diagnostics)
	return result
}

func (v *View) Binding() qimu.BindingSnapshot { return v.use.Binding() }

func (v *View) WithReader(ctx context.Context, read func(context.Context, io.Reader) error) error {
	if read == nil {
		return errors.New("authoring: resource reader is required")
	}
	work, err := v.use.Lifetime().Enter(ctx)
	if err != nil {
		return err
	}
	defer work.Finish()
	return read(work.Context(), io.NewSectionReader(v.file, 0, v.size))
}

func (v *View) Close(ctx context.Context) error {
	_, err := v.use.Close(ctx)
	return err
}
