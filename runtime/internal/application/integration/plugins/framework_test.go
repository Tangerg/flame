package plugins

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"

	"github.com/samber/lo"
)

const planningBackendProfile frame.Profile = "test-backend"

type planningDriver struct{ profile frame.Profile }

func (d planningDriver) Profile() frame.Profile { return d.profile }
func (d planningDriver) RealmBound() bool       { return d.profile == frame.BrowserJS }
func (planningDriver) Prepare(context.Context, *frame.Setup) ([]frame.ExportValue, error) {
	return nil, errors.New("planning fixture must not execute a backend")
}

// testFramework seeds the real Host from immutable fixture declarations. The
// fake storage port only records Host-admitted changes; it cannot select or
// advance a second installation authority.
func testFramework(t testing.TB, store Store, dependencies ...*installationDependencies) *frame.Host {
	t.Helper()
	admission := frame.NewAdmission()
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
	config := frame.HostConfig{Admission: admission, Limits: frame.HostLimits{MaxInstallations: 128}, Drivers: []frame.Driver{planningDriver{planningBackendProfile}, planningDriver{frame.BrowserJS}, planningDriver{frame.StaticResource}}}
	if memory != nil {
		config.Installations = &frameworkTestStore{memory: memory, admission: admission, observed: observed}
	}
	if observed != nil {
		config.UseOwners = []frame.UseOwner{&frameworkTestUseOwner{observed: observed, memory: memory}}
	}
	host, err := frame.NewHost(context.Background(), config)
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
	var records []frame.InstallationRecord
	var definitions []*frame.Plugin
	for _, snapshot := range snapshots {
		projection, err := ProjectFrameworkRelease(snapshot.Selected, planningBackendProfile)
		if err != nil {
			t.Fatal(err)
		}
		if err := RegisterFrameworkRelease(host, projection); err != nil {
			t.Fatal(err)
		}
		configuration, err := EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, frame.InstallationRecord{ID: snapshot.Installation.ID().Host(), Release: projection.Plugin.Revision(), Configuration: configuration, Enabled: snapshot.Installation.Enabled(), Units: projection.Plugin.Units()})
		definitions = append(definitions, projection.Plugin)
	}
	if err := host.Restore(context.Background(), records, definitions); err != nil {
		t.Fatal(err)
	}
	return host
}

type frameworkTestUseOwner struct {
	observed *installationDependencies
	memory   *installationMemory
}

func (*frameworkTestUseOwner) ID() string { return "fixture.business" }
func (o *frameworkTestUseOwner) Held(_ context.Context, id frame.InstallationID) ([]frame.Dependency, error) {
	if !o.observed.inUse || o.memory == nil || o.memory.record == nil || o.memory.record.ID.String() != id.String() {
		return nil, nil
	}
	return []frame.Dependency{{InstallationID: id, Release: o.memory.record.Selected}}, nil
}

type frameworkTestStore struct {
	memory    *installationMemory
	admission *frame.Admission
	observed  *installationDependencies
}

func (s *frameworkTestStore) CommitInstallation(ctx context.Context, change frame.InstallationChange) (frame.CommitState, error) {
	if err := s.admission.CheckAdmitted(ctx); err != nil {
		return frame.CommitRejected, err
	}
	if change.After == nil {
		if s.memory.removeErr != nil {
			return frame.CommitRejected, s.memory.removeErr
		}
		s.memory.record = nil
	} else {
		if s.memory.saveErr != nil {
			return frame.CommitRejected, s.memory.saveErr
		}
		record, err := DecodeFrameworkConfiguration(*change.After)
		if err != nil {
			return frame.CommitRejected, err
		}
		if s.memory.record != nil && s.memory.record.Selected == record.Selected {
			record.Staged = s.memory.record.Staged
		}
		s.memory.record = &record
	}
	if s.observed != nil {
		s.observed.executionChanged = change.Before != nil && (change.After == nil || change.Before.Enabled != change.After.Enabled || change.Before.Release != change.After.Release || !bytes.Equal(change.Before.Configuration, change.After.Configuration))
	}
	return frame.CommitSucceeded, nil
}
func (s *frameworkTestStore) ConfirmInstallation(ctx context.Context, _ frame.InstallationID) error {
	return s.admission.CheckAdmitted(ctx)
}
func (s *frameworkTestStore) Installations(context.Context) ([]frame.InstallationRecord, error) {
	return nil, errors.New("fixture restore is explicitly seeded")
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
