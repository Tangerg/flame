package plugins

import (
	"encoding/json/jsontext"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func BenchmarkInstalledPointResolution(b *testing.B) {
	for _, composition := range []portable.Composition{portable.Collection, portable.Decoration} {
		for _, count := range []int{1, 8, 32} {
			b.Run(fmt.Sprintf("%s/members=%d", composition, count), func(b *testing.B) {
				contract, err := portable.AdmitPoint(portable.PointDescriptor{Protocol: portable.Protocol, Reference: "knowledge.sources", Composition: composition, Scope: extension.ScopeInstallation, Optional: true, Ordering: portable.ExplicitList, Value: jsontext.Value(`{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}`), Applicability: jsontext.Value(`{"type":"object"}`)})
				if err != nil {
					b.Fatal(err)
				}
				makeSnapshot := func(name string, declaration plugin.Declaration) plugin.Snapshot {
					declaration.Name = name
					release, err := plugin.NewRelease(testsupport.Digest(name), declaration)
					if err != nil {
						b.Fatal(err)
					}
					installation, err := plugin.New(testsupport.InstallationID(b), name, release)
					if err != nil {
						b.Fatal(err)
					}
					if err := installation.Enable(release); err != nil {
						b.Fatal(err)
					}
					return plugin.Snapshot{Installation: installation, Selected: release}
				}
				consumer := makeSnapshot("knowledge", plugin.Declaration{Points: []plugin.Point{{ID: "sources", Contract: contract.Identity()}}})
				store := requirementInstallations{snapshots: []plugin.Snapshot{consumer}}
				order := make([]PointReference, count)
				for i := range count {
					value, err := jsonvalue.Encode(struct {
						Title string `json:"title"`
					}{Title: fmt.Sprintf("Source %d", i)})
					if err != nil {
						b.Fatal(err)
					}
					producer := makeSnapshot(fmt.Sprintf("source-%d", i), plugin.Declaration{PointValues: []plugin.PointValue{{ID: "source", Contract: contract.Identity(), Value: value}}})
					store.snapshots = append(store.snapshots, producer)
					order[i] = PointReference{Installation: producer.Installation.ID(), Contribution: "source"}
				}
				context := portable.PointContext{Target: portable.Target{Scope: extension.ScopeInstallation, Reference: consumer.Installation.ID().String()}, Attributes: jsontext.Value(`{}`)}
				host := testFramework(b, store)
				b.ReportAllocs()
				for b.Loop() {
					bindings, err := ResolvePoint(b.Context(), host, store, contract, context, order)
					if err != nil || len(bindings) != count {
						b.Fatalf("point resolution: %d, %v", len(bindings), err)
					}
				}
			})
		}
	}
}
