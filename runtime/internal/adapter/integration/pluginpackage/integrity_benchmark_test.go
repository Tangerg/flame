package pluginpackage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func benchmarkRelease(b *testing.B, files map[string]string) (*Releases, plugin.Release) {
	b.Helper()
	source := b.TempDir()
	for name, content := range files {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
	}
	db, err := sqlite.Open(b.Context(), filepath.Join(b.TempDir(), "flame.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	directory := b.TempDir()
	b.Cleanup(func() {
		_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	releases, err := New(directory, sqlite.NewReleaseStore(db))
	if err != nil {
		b.Fatal(err)
	}
	release, err := publishPackage(b.Context(), releases, source)
	if err != nil {
		b.Fatal(err)
	}
	return releases, release
}

// BenchmarkLaunchIntegrity compares the full scan every connection used to pay
// with the launch check of an unchanged release.
func BenchmarkLaunchIntegrity(b *testing.B) {
	files := map[string]string{"plugin.json": portableManifest}
	for index := range 16 {
		files[fmt.Sprintf("data-%d.bin", index)] = strings.Repeat("x", 4<<20)
	}
	for index := range 512 {
		files[fmt.Sprintf("lib/module-%d.js", index)] = strings.Repeat("y", 4<<10)
	}
	releases, release := benchmarkRelease(b, files)
	b.Run("full scan", func(b *testing.B) {
		for b.Loop() {
			if _, err := releases.verify(b.Context(), release.Digest()); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("unchanged launch", func(b *testing.B) {
		for b.Loop() {
			root, err := releases.currentRoot(b.Context(), release.Digest())
			if err != nil {
				b.Fatal(err)
			}
			if err := root.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkProofFootprint measures the heap a cached proof retains for
// dependency-shaped trees and checks that the estimate bounding the integrity
// cache does not undercount it.
func BenchmarkProofFootprint(b *testing.B) {
	for _, count := range []int{500, 1000, 2000, 3500} {
		b.Run(fmt.Sprintf("%d files", count), func(b *testing.B) {
			files := map[string]string{"plugin.json": portableManifest}
			for index := range count - 1 {
				files[fmt.Sprintf("node_modules/package-%d/lib/module-%d.js", index/20, index)] = strings.Repeat("y", 64)
			}
			releases, release := benchmarkRelease(b, files)
			dir, err := releases.Root(release.Digest())
			if err != nil {
				b.Fatal(err)
			}
			var retained, estimated, entries int64
			for b.Loop() {
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				proof, err := verifyTree(b.Context(), dir, release.Digest())
				if err != nil {
					b.Fatal(err)
				}
				runtime.GC()
				runtime.ReadMemStats(&after)
				retained = int64(after.HeapAlloc) - int64(before.HeapAlloc)
				estimated = proof.estimate()
				entries = int64(len(proof.stamps))
				runtime.KeepAlive(proof)
			}
			if estimated < retained {
				b.Fatalf("estimate %d B undercounts the %d B a proof retains", estimated, retained)
			}
			b.ReportMetric(float64(retained)/float64(entries), "retained-B/entry")
			b.ReportMetric(float64(estimated)/float64(entries), "estimated-B/entry")
		})
	}
}
