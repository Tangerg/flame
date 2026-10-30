package fileobservation

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDirectoryObservationTracksSymlinkMetadataWithoutFollowing(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "target")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	changed := make(chan []string, 8)
	observation, err := Watch([]Target{{Key: "directory", Path: root, Boundary: root, MaxEntries: 1, MaxBytes: testMaxBytes}}, func(keys []string) { changed <- keys }, discardOutage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := observation.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := unix.Lutimes(link, []unix.Timeval{{Sec: 1}, {Sec: 1}}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, changed, "directory")
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("observing link metadata touched its external target")
	}
}
