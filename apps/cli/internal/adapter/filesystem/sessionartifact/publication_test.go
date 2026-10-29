package sessionartifact

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem/commit"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/spf13/fileflow"
)

func TestPublicationRetainsTheSelectedPathAndEachFailure(t *testing.T) {
	for _, phase := range []string{"before publication", "confirmation", "cleanup", "cleanup and confirmation", "success"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			destination := filepath.Join(root, "archive.json")
			if err := os.WriteFile(destination, []byte("existing content"), 0o600); err != nil {
				t.Fatal(err)
			}
			staged, err := stage(root, []byte("new content"))
			if err != nil {
				t.Fatal(err)
			}
			publishFailure := errors.New("publication cleanup failed")
			confirmFailure := errors.New("directory sync failed")
			flow := fileflow.Flow{FindAvailableName: fileflow.FindAvailableNameAuto, NoCreateDirs: true}
			confirmations := 0
			result := publishStaged(func() (string, error) {
				if phase == "before publication" {
					return "", publishFailure
				}
				path, err := flow.Rename(staged, destination)
				if err != nil {
					t.Fatal(err)
				}
				if phase == "cleanup" || phase == "cleanup and confirmation" {
					return path, &fileflow.ErrFailedRemovingOriginal{Err: publishFailure, File: staged}
				}
				return path, nil
			}, func(path string) error {
				confirmations++
				if phase == "confirmation" || phase == "cleanup and confirmation" {
					return confirmFailure
				}
				return confirmPublication(path)
			})
			if phase == "before publication" {
				if result.Committed() || result.Path != "" || confirmations != 0 || !errors.Is(result.Err, publishFailure) {
					t.Fatalf("uncommitted result = %+v", result)
				}
				return
			}
			if !result.Committed() || result.Path == "" || result.Path == destination || confirmations != 1 {
				t.Fatalf("published result = %+v, confirmations=%d", result, confirmations)
			}
			body, err := os.ReadFile(result.Path)
			if err != nil || string(body) != "new content" {
				t.Fatalf("selected file = %q, %v", body, err)
			}
			original, err := os.ReadFile(destination)
			if err != nil || string(original) != "existing content" {
				t.Fatalf("original was overwritten: %q, %v", original, err)
			}
			unconfirmed := phase == "confirmation" || phase == "cleanup and confirmation"
			if (result.State == commit.Visible) != unconfirmed || errors.Is(result.Err, confirmFailure) != unconfirmed {
				t.Fatalf("durability evidence = %+v", result)
			}
			cleanup := phase == "cleanup" || phase == "cleanup and confirmation"
			if errors.Is(result.Err, publishFailure) != cleanup {
				t.Fatalf("cleanup evidence = %+v", result)
			}
			if phase == "success" && result.Err != nil {
				t.Fatal(result.Err)
			}
		})
	}
}

func TestIdenticalPublicationConfirmsTheExistingFile(t *testing.T) {
	root := t.TempDir()
	document, err := session.NewDocument(protocol.ExportFormatMarkdown, []byte("# Session"))
	if err != nil {
		t.Fatal(err)
	}
	first := (Store{}).Publish(root, "Session", "session.md", document)
	if first.Err != nil || first.State != commit.Durable {
		t.Fatalf("first publication = %+v", first)
	}
	before, err := os.Stat(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second := (Store{}).Publish(root, "Session", "session.md", document)
	if second.Err != nil || second.State != commit.Durable || second.Path != first.Path {
		t.Fatalf("identical publication = %+v", second)
	}
	after, err := os.Stat(second.Path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("identical publication replaced its inode: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging cleanup = %v, %v", entries, err)
	}
}
