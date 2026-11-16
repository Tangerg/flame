package plugins

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

const planningBackendProfile framehost.Profile = "test-backend"

type planningDriver struct{ profile framehost.Profile }

func (d planningDriver) Profile() framehost.Profile { return d.profile }
func (d planningDriver) RealmBound() bool           { return d.profile == framehost.BrowserJS }
func (planningDriver) Prepare(context.Context, framehost.Setup) ([]framehost.ExportValue, error) {
	return nil, errors.New("planning fixture must not execute a backend")
}

// testFramework seeds the real Host from immutable fixture declarations. The
// fake storage port only records Host-admitted changes; it cannot select or
// advance a second installation authority.
func testFramework(t testing.TB, store Store, dependencies ...*installationDependencies) framehost.Host {
	t.Helper()
	admission := framehost.NewAdmission()
	var memory *installationMemory
	switch value := store.(type) {
	case *installationMemory:
		memory = value
	}
	var observed *installationDependencies
	if len(dependencies) > 0 {
		observed = dependencies[0]
		observed.admission = admission
	}
	config := framehost.Config{Authority: admission, Limits: framehost.Limits{MaxInstallations: 128}, Drivers: []framehost.Driver{planningDriver{planningBackendProfile}, planningDriver{framehost.BrowserJS}, planningDriver{framehost.StaticResource}}}
	if !lo.IsNil(store) {
		config.Authority = &frameworkTestStore{store: store, memory: memory, admission: admission, observed: observed}
	}
	if observed != nil {
		config.UseOwners = []framehost.UseOwner{&frameworkTestUseOwner{observed: observed, memory: memory}}
	}
	host, err := framehost.New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if lo.IsNil(store) {
		return host
	}
	var snapshots []plugin.Snapshot
	if memory != nil && memory.getErr != nil {
		return host
	}
	if memory != nil {
		snapshots, err = memory.List(context.Background())
	} else {
		snapshots, err = store.List(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	var definitions []*framehost.Plugin
	for _, snapshot := range snapshots {
		projection, err := ProjectFrameworkRelease(snapshot.Selected, planningBackendProfile)
		if err != nil {
			t.Fatal(err)
		}
		if err := RegisterFrameworkRelease(host, projection); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, projection.Plugin)
	}
	if err := host.Restore(context.Background(), definitions); err != nil {
		t.Fatal(err)
	}
	return host
}

type frameworkTestUseOwner struct {
	observed *installationDependencies
	memory   *installationMemory
}

func (*frameworkTestUseOwner) ID() string { return "fixture.business" }
func (o *frameworkTestUseOwner) Held(_ context.Context, id framehost.InstallationID) ([]framehost.Dependency, error) {
	if !o.observed.inUse || o.memory == nil || o.memory.record == nil || o.memory.record.ID.String() != id.String() {
		return nil, nil
	}
	return []framehost.Dependency{{InstallationID: id, Release: o.memory.record.Selected}}, nil
}

type frameworkTestStore struct {
	store     Store
	memory    *installationMemory
	admission framehost.Admission
	observed  *installationDependencies
}

func (s *frameworkTestStore) CommitInstallation(ctx context.Context, change framehost.InstallationChange) (framehost.CommitState, error) {
	if err := s.admission.CheckAdmitted(ctx); err != nil {
		return framehost.NotCommitted, err
	}
	if change.After == nil {
		if s.memory.removeErr != nil {
			return framehost.NotCommitted, s.memory.removeErr
		}
		s.memory.record = nil
	} else {
		if s.memory.saveErr != nil {
			return framehost.NotCommitted, s.memory.saveErr
		}
		record, err := DecodeFrameworkConfiguration(*change.After)
		if err != nil {
			return framehost.NotCommitted, err
		}
		if s.memory.record != nil && s.memory.record.Selected == record.Selected {
			record.Staged = s.memory.record.Staged
		}
		s.memory.record = &record
	}
	if s.observed != nil {
		s.observed.executionChanged = change.Before != nil && (change.After == nil || change.Before.Enabled != change.After.Enabled || change.Before.Release != change.After.Release || !bytes.Equal(change.Before.Configuration, change.After.Configuration))
	}
	return framehost.Committed, nil
}
func (s *frameworkTestStore) ConfirmInstallation(ctx context.Context, _ framehost.InstallationID) error {
	return s.admission.CheckAdmitted(ctx)
}
func (s *frameworkTestStore) Admission() framehost.Admission { return s.admission }
func (*frameworkTestStore) Epoch() uint64                    { return 1 }
func (*frameworkTestStore) ApproveUse(context.Context, framehost.UseApproval) (framehost.CommitState, error) {
	return framehost.NotCommitted, errors.New("fixture does not support surviving execution")
}
func (*frameworkTestStore) SettleUse(context.Context, string, frame.ReleaseProof) error {
	return errors.New("fixture did not approve surviving execution")
}
func (s *frameworkTestStore) Installations(ctx context.Context) ([]framehost.InstallationRecord, error) {
	snapshots, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]framehost.InstallationRecord, 0, len(snapshots))
	for _, snapshot := range snapshots {
		configuration, err := EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
		if err != nil {
			return nil, err
		}
		records = append(records, framehost.InstallationRecord{ID: snapshot.Installation.ID().Host(), Release: snapshot.Selected.Digest(), Configuration: configuration, Enabled: snapshot.Installation.Enabled()})
	}
	return records, nil
}

func (s *installationMemory) Stage(_ context.Context, id identity.InstallationID, digest framevalue.Digest) error {
	if s.record == nil || s.record.ID != id {
		return plugin.ErrNotFound
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	s.record.Staged = &digest
	return nil
}
