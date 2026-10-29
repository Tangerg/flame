package statefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type publicationFaultFile struct {
	*os.File
	fail   string
	cause  error
	closes int
}

func (f *publicationFaultFile) Write(body []byte) (int, error) {
	if f.fail == "write" {
		n, err := f.File.Write(body[:len(body)/2])
		return n, errors.Join(f.cause, err)
	}
	return f.File.Write(body)
}

func (f *publicationFaultFile) Sync() error {
	if f.fail == "file sync" {
		return f.cause
	}
	return f.File.Sync()
}

func (f *publicationFaultFile) Close() error {
	f.closes++
	err := f.File.Close()
	if f.fail == "close" {
		return errors.Join(f.cause, err)
	}
	return err
}

func TestSnapshotPublicationKeepsCommitAndDurabilityEvidence(t *testing.T) {
	for _, stage := range []string{"write", "file sync", "close", "rename", "directory sync", "success"} {
		t.Run(stage, func(t *testing.T) {
			path := t.TempDir()
			root, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := root.WriteFile("record.json", []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			temporary, name, err := createTemporary(root)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("injected " + stage + " refusal")
			file := &publicationFaultFile{File: temporary, fail: stage, cause: cause}
			confirmations := 0
			result := publishSnapshot(file, []byte("replacement"), func() error {
				if stage == "rename" {
					return cause
				}
				return root.Rename(name, "record.json")
			}, func() error {
				confirmations++
				if stage == "directory sync" {
					return cause
				}
				return syncDirectoryAt(root, ".")
			})
			wantState := filesystem.CommitUncommitted
			wantBody, wantConfirmations := "original", 0
			if stage == "directory sync" || stage == "success" {
				wantState = filesystem.CommitVisible
				wantBody, wantConfirmations = "replacement", 1
			}
			if stage == "success" {
				wantState = filesystem.CommitDurable
			}
			if result.State != wantState || file.closes != 1 || confirmations != wantConfirmations {
				t.Fatalf("publication = %+v, closes=%d, confirmations=%d", result, file.closes, confirmations)
			}
			if stage == "success" {
				if result.Err != nil {
					t.Fatal(result.Err)
				}
			} else if !errors.Is(result.Err, cause) {
				t.Fatalf("publication lost original cause: %v", result.Err)
			}
			body, err := os.ReadFile(filepath.Join(path, "record.json"))
			if err != nil || string(body) != wantBody {
				t.Fatalf("visible record = %q, %v; want %q", body, err, wantBody)
			}
		})
	}
}

func TestDurabilityConfirmationUsesTheOwnedPhysicalDirectory(t *testing.T) {
	parent := t.TempDir()
	boundary := filepath.Join(parent, "owner")
	store := openStore(t, filepath.Join(boundary, "state"))
	if result := store.Replace(filepath.Join("sessions", "record.json"), []byte("record")); result.Err != nil {
		t.Fatal(result.Err)
	}
	if err := store.ConfirmDurability(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(boundary, filepath.Join(parent, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(boundary, "state"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmDurability(); err == nil {
		t.Fatal("confirmation adopted a replacement physical boundary")
	}
	if result := store.Replace("new.json", []byte("wrong owner")); result.State != filesystem.CommitUncommitted || result.Err == nil {
		t.Fatalf("replacement physical boundary admitted write: %+v", result)
	}
}
