package qimu_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

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
