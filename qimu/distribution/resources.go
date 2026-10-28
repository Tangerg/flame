package distribution

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
)

// Resources exposes only a unit's declared immutable files. Reads return
// detached bytes; the provider's original lease fences access after retirement.
type Resources struct {
	lease *Lease
	files []string
}

func (r *Resources) Files() []string { return slices.Clone(r.files) }

func (r *Resources) Read(ctx context.Context, path string) ([]byte, error) {
	if r == nil || !slices.Contains(r.files, path) {
		return nil, ErrInvalid
	}
	return r.lease.Read(ctx, path)
}

type ResourceDriver struct{ store *Store }

func NewResourceDriver(store *Store) (*ResourceDriver, error) {
	if store == nil {
		return nil, ErrInvalid
	}
	return &ResourceDriver{store: store}, nil
}

func (*ResourceDriver) Profile() qimu.Profile { return qimu.StaticResource }

func (d *ResourceDriver) Prepare(ctx context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
	if setup == nil {
		return nil, ErrInvalid
	}
	lease, err := d.store.Acquire(ctx, setup.Binding().Source.Release)
	if err != nil {
		return nil, err
	}
	if _, err := setup.Cleanup("static content", lease.Close); err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	declaration := setup.Unit()
	var declared *Unit
	for _, unit := range lease.release.Manifest.Units {
		if unit.ID == declaration.ID {
			copy := unit
			declared = &copy
			break
		}
	}
	if declared == nil || declared.Profile != qimu.StaticResource || len(declaration.Imports) != 0 || !slices.Equal(declared.Exports, declaration.Exports) {
		return nil, qimu.ErrContractMismatch
	}
	value := &Resources{lease: lease, files: slices.Clone(declared.Resources)}
	values := make([]qimu.ExportValue, 0, len(declaration.Exports))
	for _, export := range declaration.Exports {
		if export.Contract.Mode != qimu.NativeMode {
			return nil, qimu.ErrContractMismatch
		}
		var metadata any
		for _, contribution := range lease.release.Manifest.Contributions {
			if contribution.Unit == declaration.ID && contribution.Export == export.Name {
				if contribution.Point != export.Contract || metadata != nil {
					return nil, qimu.ErrContractMismatch
				}
				admitted, err := jsonvalue.Parse(contribution.Metadata)
				if err != nil {
					return nil, err
				}
				metadata = admitted
			}
		}
		values = append(values, qimu.ExportValue{Name: export.Name, Contract: export.Contract, Metadata: metadata, Value: value, Durable: true})
	}
	return values, nil
}
