package distribution

import (
	json "encoding/json/v2"
	"errors"
	"testing"

	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/integrity"
)

func TestContractRoleIsPartOfNominalIdentity(t *testing.T) {
	service := qimu.ContractRef{Name: "example/report", Version: "1", Digest: integrity.Hash([]byte("service")), Kind: qimu.ServiceKind, Mode: qimu.PortableMode}
	point := service
	point.Kind, point.Digest = qimu.PointKind, integrity.Hash([]byte("point"))
	manifest := Manifest{SchemaVersion: 1, Package: "example.report", Version: "1", Contracts: []qimu.ContractRef{service, point}, Units: []Unit{{ID: "report", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "report", Contract: point}}}}, Contributions: []Contribution{{Unit: "report", Export: "report", Point: point, Key: "report"}}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("service and consuming point cannot share their nominal name: %v", err)
	}
	changed := service
	changed.Digest = integrity.Hash([]byte("changed service"))
	manifest.Contracts = append(manifest.Contracts, changed)
	if err := manifest.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("one nominal service version admitted two descriptors: %v", err)
	}
	manifest.Contracts = manifest.Contracts[:2]
	manifest.Units[0].Exports[0].Contract = service
	if err := manifest.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a service export was relabeled as a point contribution: %v", err)
	}
}

func TestExplicitNullContributionMetadataSurvivesAuthorEncoding(t *testing.T) {
	point := qimu.ContractRef{Name: "example/actions", Version: "1", Digest: integrity.Hash([]byte("point")), Kind: qimu.PointKind, Mode: qimu.PortableMode}
	manifest := Manifest{SchemaVersion: 1, Package: "example.actions", Version: "1", Artifacts: []Artifact{}, Contracts: []qimu.ContractRef{point}, Units: []Unit{{ID: "actions", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "action", Contract: point}}}}, Contributions: []Contribution{{Unit: "actions", Export: "action", Point: point, Key: "action", Metadata: []byte("null")}}}
	original, err := manifest.Identity()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := decoded.Identity()
	if err != nil || original != actual || string(decoded.Contributions[0].Metadata) != "null" {
		t.Fatalf("the author writer changed explicit null to omission: %s, %v", encoded, err)
	}
}

func TestThirdPartyProcessProfileUsesPortableExecutableVariants(t *testing.T) {
	manifest := Manifest{SchemaVersion: ManifestSchemaVersion, Package: "example.pipe", Version: "1", Artifacts: []Artifact{{Path: "bin/report", Executable: true, Digest: integrity.Hash([]byte("report")), Size: 6}}, Units: []Unit{{ID: "render", Profile: "report-stdio-v1", Variants: []Variant{{OS: "linux", Arch: "amd64", Path: "bin/report"}}}}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest.Units[0].Variants = nil
	if err := manifest.Validate(); err == nil {
		t.Fatal("process without executable entry accepted")
	}
}
