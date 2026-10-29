package persistence

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestReleaseReadCannotRetainRolledBackAdmission(t *testing.T) {
	db := openStore(t)
	catalog := newReleaseStore(db)
	release := testsupport.Release(t, "release", plugin.Declaration{Name: "package"})
	abort := errors.New("abort admission")
	err := sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
		if err := catalog.Admit(ctx, release); err != nil {
			return err
		}
		if err := sqlite.RunReadSnapshot(ctx, db, func(ctx context.Context) error {
			requireStoredRelease(t, ctx, catalog, release)
			return nil
		}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if _, err := catalog.Get(t.Context(), release.Digest()); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("rolled-back admission remained visible: %v", err)
	}
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	requireStoredRelease(t, t.Context(), catalog, release)
}

func TestReleaseReadFollowsRemovalAndReplacement(t *testing.T) {
	db := openStore(t)
	catalog := newReleaseStore(db)
	original := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Description: "original"})
	replacement := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Description: "replacement"})
	if err := catalog.Admit(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	requireStoredRelease(t, t.Context(), catalog, original)
	abort := errors.New("abort replacement")
	err := sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
		if err := catalog.Remove(ctx, original.Digest()); err != nil {
			return err
		}
		if _, err := catalog.Get(ctx, original.Digest()); !errors.Is(err, plugin.ErrNotFound) {
			t.Fatalf("transaction read ignored its removal: %v", err)
		}
		if err := catalog.Admit(ctx, replacement); err != nil {
			return err
		}
		requireStoredRelease(t, ctx, catalog, replacement)
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	requireStoredRelease(t, t.Context(), catalog, original)
	if err := catalog.Remove(t.Context(), original.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Get(t.Context(), original.Digest()); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("committed removal remained visible: %v", err)
	}
	if err := catalog.Admit(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	requireStoredRelease(t, t.Context(), catalog, replacement)
}

func TestReleaseReadUsesTransactionSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	readerDB, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readerDB.Close() })
	writerDB, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writerDB.Close() })
	reader, writer := newReleaseStore(readerDB), newReleaseStore(writerDB)
	original := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Description: "original"})
	replacement := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Description: "replacement"})
	if err := writer.Admit(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.RunReadSnapshot(t.Context(), readerDB, func(ctx context.Context) error {
		requireStoredRelease(t, ctx, reader, original)
		if err := sqlite.RunInTx(t.Context(), writerDB, func(ctx context.Context) error {
			if err := writer.Remove(ctx, original.Digest()); err != nil {
				return err
			}
			return writer.Admit(ctx, replacement)
		}); err != nil {
			return err
		}
		requireStoredRelease(t, ctx, reader, original)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requireStoredRelease(t, t.Context(), reader, replacement)
}

func TestReleaseReadRejectsChangedInvalidDeclaration(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "unknown storage member", body: `{"Name":"package","unexpected":true}`},
		{name: "invalid domain declaration", body: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openStore(t)
			catalog := newReleaseStore(db)
			original := testsupport.Release(t, "release", plugin.Declaration{Name: "package"})
			replacement := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Version: "2"})
			if err := catalog.Admit(t.Context(), original); err != nil {
				t.Fatal(err)
			}
			requireStoredRelease(t, t.Context(), catalog, original)
			abort := errors.New("abort invalid declaration")
			err := sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
				if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE plugin_releases SET declaration=? WHERE digest=?`, test.body, original.Digest().String()); err != nil {
					return err
				}
				if _, err := catalog.Get(ctx, original.Digest()); err == nil {
					t.Fatal("previous admission concealed invalid stored declaration")
				}
				if err := catalog.Remove(ctx, original.Digest()); err != nil {
					return err
				}
				if err := catalog.Admit(ctx, replacement); err != nil {
					return err
				}
				requireStoredRelease(t, ctx, catalog, replacement)
				return abort
			})
			if !errors.Is(err, abort) {
				t.Fatal(err)
			}
			requireStoredRelease(t, t.Context(), catalog, original)
		})
	}
}

func TestReleaseReadsDoNotShareMutableDeclarations(t *testing.T) {
	db := openStore(t)
	catalog := newReleaseStore(db)
	contract, err := portable.NewCapability(
		portable.CapabilityConfig{Reference: "consumer.role", Composition: portable.Selection, Scope: extension.ScopeInstallation},
		portable.MethodDescriptor{Name: "owner.read", Kind: portable.Query, Target: extension.ScopeInstallation, Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := testsupport.ServerName("backend")
	release := testsupport.Release(t, "release", plugin.Declaration{
		Name: "package",
		Servers: []plugin.Server{
			{Name: backend, Transport: mcpserver.TransportStdio, Command: "backend", Args: []string{"serve"}, Env: map[string]string{"MODE": "read"}},
			{Name: testsupport.ServerName("remote"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp", Headers: map[string]string{"X-Mode": "read"}},
			{Name: testsupport.ServerName("managed"), Transport: mcpserver.TransportStdio, Managed: &plugin.ManagedBackend{Runtime: "node", Entry: "backend/main.mjs"}},
		},
		Contributions: []plugin.Contribution{{ID: "reader", Server: backend, Export: "read", Contract: contract.Identity(), Requires: []plugin.Requirement{{ID: "collaborator", Contract: contract.Identity(), Optional: true}}}},
		Components: []plugin.ClientComponent{{
			ID: "markdown", Title: "Markdown", Scope: extension.ComponentScopeFile, Placement: extension.ComponentPlacementFileRenderer,
			Entry: plugin.Namespace + "/components/markdown.js", Export: "Markdown", Selector: &plugin.RendererSelector{Kind: extension.RendererFileExtension, Name: "md"},
		}},
	})
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	stored, err := catalog.Get(t.Context(), release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	declaration := stored.Declaration()
	declaration.Name = "changed"
	declaration.Servers[0].Args[0] = "changed"
	declaration.Servers[0].Env["MODE"] = "changed"
	declaration.Servers[1].Headers["X-Mode"] = "changed"
	declaration.Servers[2].Managed.Entry = "changed.mjs"
	declaration.Contributions[0].Requires[0].ID = "changed"
	declaration.Components[0].Selector.Name = "changed"
	contribution, found := stored.Contribution("reader")
	if !found {
		t.Fatal("stored release lost its contribution")
	}
	contribution.Requires[0].ID = "changed"
	requireStoredRelease(t, t.Context(), catalog, release)
	installations := newInstallationStore(catalog)
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installations.projectRecord(t.Context(), installation.Snapshot()); err != nil {
		t.Fatal(err)
	}
	restored, found, err := installations.Get(t.Context(), installation.ID())
	if err != nil || !found {
		t.Fatalf("installation restoration = (%t, %v)", found, err)
	}
	if !reflect.DeepEqual(restored.Selected.Declaration(), release.Declaration()) {
		t.Fatal("caller mutation reached the installation's admitted release")
	}
}

func TestReleaseReadsConcurrentlyPreserveAdmission(t *testing.T) {
	db := openStore(t)
	catalog := newReleaseStore(db)
	release := testsupport.Release(t, "release", plugin.Declaration{
		Name: "package",
		Servers: []plugin.Server{{
			Name: testsupport.ServerName("backend"), Transport: mcpserver.TransportStdio, Command: "backend", Args: []string{"serve"},
		}},
	})
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	const readers = 8
	var group sync.WaitGroup
	for range readers {
		group.Go(func() {
			stored, err := catalog.Get(t.Context(), release.Digest())
			if err != nil {
				t.Error(err)
				return
			}
			declaration := stored.Declaration()
			if !reflect.DeepEqual(declaration, release.Declaration()) {
				t.Error("concurrent read changed the admitted declaration")
			}
			declaration.Servers[0].Args[0] = "changed"
		})
	}
	group.Wait()
	requireStoredRelease(t, t.Context(), catalog, release)
}

func requireStoredRelease(t *testing.T, ctx context.Context, catalog *ReleaseStore, want plugin.Release) {
	t.Helper()
	got, err := catalog.Get(ctx, want.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest() != want.Digest() || !reflect.DeepEqual(got.Declaration(), want.Declaration()) {
		t.Fatalf("stored release = %+v, want %+v", got.Declaration(), want.Declaration())
	}
}
