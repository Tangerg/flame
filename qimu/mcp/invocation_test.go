package mcp

import (
	"testing"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestInvocationMetadataRefusesMalformedAndCompetingOwners(t *testing.T) {
	owner := portable.Invocation{ID: "original", Origin: portable.Origin("model"), Parent: "parent", Target: portable.Target{Scope: "installation", Reference: "producer"}}
	ctx, err := WithInvocation(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{`,
		`null`,
		`{"id":"original","origin":"model","parent":"parent","target":{"scope":"installation","reference":"producer"},"unknown":true}`,
		`{"id":"original","id":"replacement","origin":"model","target":{"scope":"installation","reference":"producer"}}`,
		`{"id":"replacement","origin":"model","parent":"parent","target":{"scope":"installation","reference":"producer"}}`,
		`{"id":"original","origin":"human","parent":"parent","target":{"scope":"installation","reference":"producer"}}`,
		`{"id":"original","origin":"model","parent":"replacement","target":{"scope":"installation","reference":"producer"}}`,
		`{"id":"original","origin":"model","parent":"parent","target":{"scope":"installation","reference":"another-producer"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			meta := sdk.Meta{InvocationMetadataKey: []byte(raw)}
			if _, err := invocationMetadata(ctx, sdk.ParamsBase{Meta: meta}); err == nil {
				t.Fatal("outbound metadata replaced its calling owner")
			}
			if _, err := bindInvocationMetadata(ctx, meta, nil); err == nil {
				t.Fatal("inbound metadata replaced its calling owner")
			}
		})
	}
	method, err := NewMethod[struct{}, struct{}](MethodConfig{Name: "owner.read", Kind: portable.Query, Target: "workspace", Description: "Read the captured workspace."})
	if err != nil {
		t.Fatal(err)
	}
	var meta sdk.Meta
	if err := meta.Set(InvocationMetadataKey, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := bindInvocationMetadata(t.Context(), meta, method.contract); err == nil {
		t.Fatal("installation metadata entered a workspace method")
	}
	if _, err := InvocationFromContext(t.Context()); err == nil {
		t.Fatal("invocation escaped its captured context")
	}
}
