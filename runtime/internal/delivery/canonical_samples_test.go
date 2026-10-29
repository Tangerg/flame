package delivery

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func readCanonicalSample(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../contract/samples", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if err := protocol.ValidateWireTree(target); err != nil {
		t.Fatal(err)
	}
}

func canonicalArchive(t *testing.T, name string) protocol.SessionArtifact {
	t.Helper()
	if name == "session.artifact.json" {
		var artifact protocol.SessionArtifact
		readCanonicalSample(t, name, &artifact)
		return artifact
	}
	var response protocol.ExportSessionResponse
	readCanonicalSample(t, name, &response)
	if response.Artifact == nil {
		t.Fatal("published JSON export has no artifact")
	}
	return *response.Artifact
}

func TestCanonicalArchivesSatisfyProductAdmission(t *testing.T) {
	for _, name := range []string{"session.artifact.json", "method.sessions.export.resp.json"} {
		t.Run(name, func(t *testing.T) {
			portable, err := portableArtifactFromWire(canonicalArchive(t, name))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := portable.CanonicalSnapshot(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCanonicalToolItemsSatisfyTranscriptAdmission(t *testing.T) {
	for _, name := range []string{"item.toolCall.json", "item.completed.json"} {
		t.Run(name, func(t *testing.T) {
			var item protocol.ArtifactItem
			if name == "item.toolCall.json" {
				readCanonicalSample(t, name, &item)
			} else {
				var event protocol.RunEvent
				readCanonicalSample(t, name, &event)
				encoded, err := json.Marshal(event.Event.Item)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(encoded, &item, json.RejectUnknownMembers(true)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := portableItemFromArtifact("session_sample", name, item); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestArchiveAdmissionStillRejectsContradictoryTimeAndWatermarks(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*protocol.SessionArtifact)
		want   string
	}{
		{"duration", func(artifact *protocol.SessionArtifact) { artifact.Items[0].DurationMillis = new(int64(1200)) }, "duration exceeds its lifecycle"},
		{"message watermark", func(artifact *protocol.SessionArtifact) { artifact.Runs[0].MessageMark = new(4) }, "message watermark"},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := canonicalArchive(t, "session.artifact.json")
			test.change(&artifact)
			portable, err := portableArtifactFromWire(artifact)
			if err == nil {
				_, err = portable.CanonicalSnapshot()
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("admission = %v, want %q", err, test.want)
			}
		})
	}
}
