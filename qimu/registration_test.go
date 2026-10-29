package qimu_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

type absentDriver struct{ qimu.Driver }
type absentStore struct {
	qimu.InstallationStore
	qimu.ExecutionProtection
}

func TestRegistrationRejectsTypedNilCollaborators(t *testing.T) {
	host := testHost(t, qimu.HostConfig{})
	if err := host.RegisterDriver((*absentDriver)(nil)); !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("typed nil driver admitted: %v", err)
	}
	if err := host.Register((*qimu.Contract[string])(nil)); !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("typed nil declaration admitted: %v", err)
	}
	if err := host.Admission().RegisterUseOwner((*retainedUseOwner)(nil)); !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("typed nil business owner admitted: %v", err)
	}
	for _, config := range []qimu.HostConfig{
		{Installations: (*absentStore)(nil)},
		{ExecutionProtection: (*absentStore)(nil)},
		{Drivers: []qimu.Driver{(*absentDriver)(nil)}},
	} {
		if host, err := qimu.NewHost(t.Context(), config); host != nil || !errors.Is(err, qimu.ErrInvalid) {
			t.Fatalf("host retained an absent collaborator: %v, %v", host, err)
		}
	}
}

func TestFailedHostConstructionDoesNotRegisterPartialBusinessOwners(t *testing.T) {
	admission := qimu.NewAdmission()
	owner := &diagnosticUseOwner{id: "consumer.original"}
	if host, err := qimu.NewHost(t.Context(), qimu.HostConfig{Admission: admission, UseOwners: []qimu.UseOwner{owner, nil}}); host != nil || !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("invalid construction was accepted: %v, %v", host, err)
	}
	if err := admission.RegisterUseOwner(owner); err != nil {
		t.Fatalf("failed constructor left another registered authority: %v", err)
	}
}

func TestActualExportsRejectTypedNilValues(t *testing.T) {
	for _, value := range []any{(*int)(nil), (func())(nil), (map[string]string)(nil), ([]string)(nil), (chan int)(nil)} {
		host := testHost(t, qimu.HostConfig{})
		contract, err := qimu.NewContract[any]("example.present", "1")
		if err != nil {
			t.Fatal(err)
		}
		if err := host.Register(contract); err != nil {
			t.Fatal(err)
		}
		plugin := installNative(t, host, "absent-value", qimu.Unit{
			Declaration: qimu.UnitDeclaration{ID: "main", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "actual", Contract: contract.Ref()}}},
			Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
				return []qimu.ExportValue{qimu.Provide("actual", contract, value)}, nil
			},
		})
		if _, err := host.Activate(t.Context(), source("absent-value", plugin, "main", "actual", contract.Ref())); !errors.Is(err, qimu.ErrContractMismatch) {
			t.Fatalf("typed nil %T was published as an actual service: %v", value, err)
		}
	}
}

func TestContractBatchRejectsConflictsWithoutPartialRegistration(t *testing.T) {
	text, _ := qimu.NewContract[string]("example.batch", "1")
	number, _ := qimu.NewContract[int]("example.batch", "1")
	point, _ := qimu.NewPoint[string, string]("example.batch", "1")
	pointMetadata, _ := qimu.NewPoint[int, string]("example.batch", "1")
	pointValue, _ := qimu.NewPoint[string, int]("example.batch", "1")
	event, _ := qimu.NewEvent[string]("example.batch", "1")
	eventPayload, _ := qimu.NewEvent[int]("example.batch", "1")
	portable := func(descriptor string) qimu.Contract[string] {
		ref := qimu.ContractRef{Name: "example.batch", Version: "1", Kind: qimu.ServiceKind, Mode: qimu.PortableMode, Digest: qimu.Hash([]byte(descriptor))}
		value, err := qimu.BindContract[string](ref, []byte(descriptor))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, test := range []struct {
		name   string
		first  qimu.Declaration
		second qimu.Declaration
	}{
		{"native service type", text, number},
		{"point metadata type", point, pointMetadata},
		{"point value type", point, pointValue},
		{"event payload type", event, eventPayload},
		{"portable descriptor", portable(`{"method":"one"}`), portable(`{"method":"two"}`)},
		{"execution mode", text, portable(`{"method":"portable"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := testHost(t, qimu.HostConfig{})
			unrelated, _ := qimu.NewContract[string]("example.unrelated", "1")
			if err := host.Register(unrelated, test.first, test.second); !errors.Is(err, qimu.ErrContractMismatch) {
				t.Fatalf("conflicting batch was admitted: %v", err)
			}
			if _, err := host.ContractDescriptor(unrelated.Ref()); !errors.Is(err, qimu.ErrContractMismatch) {
				t.Fatalf("failed batch partially registered a declaration: %v", err)
			}
			if err := host.Register(test.second, test.second); err != nil {
				t.Fatalf("failed batch retained its first declaration: %v", err)
			}
			if err := host.Register(test.first); !errors.Is(err, qimu.ErrContractMismatch) {
				t.Fatalf("sequential registration differs from batch admission: %v", err)
			}
		})
	}
}

func TestContractKindsKeepIndependentNominalIdentities(t *testing.T) {
	host := testHost(t, qimu.HostConfig{})
	service, _ := qimu.NewContract[string]("example.same-name", "1")
	point, _ := qimu.NewPoint[string, int]("example.same-name", "1")
	event, _ := qimu.NewEvent[bool]("example.same-name", "1")
	if err := host.Register(service, point, event, service, point, event); err != nil {
		t.Fatalf("distinct contract kinds collided: %v", err)
	}
}
