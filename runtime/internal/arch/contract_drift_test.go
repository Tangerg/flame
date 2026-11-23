package arch

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func readArtifact(t *testing.T, dir, name string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, name), err)
	}
	return content
}

func TestProtocolVersionAgreesEverywhere(t *testing.T) {
	root := moduleRoot(t)

	var manifest struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if unmarshalErr := json.Unmarshal(raw, &manifest); unmarshalErr != nil {
		t.Fatalf("decode manifest: %v", unmarshalErr)
	}
	if manifest.ProtocolVersion != protocol.Version {
		t.Errorf("manifest protocol %q != code %q", manifest.ProtocolVersion, protocol.Version)
	}

	samples, err := filepath.Glob(filepath.Join(root, "contract", "samples", "*.json"))
	if err != nil {
		t.Fatalf("glob canonical samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no canonical samples found; the sweep below would pass on nothing")
	}
	stated := 0
	for _, path := range samples {
		var decoded any
		if err := json.Unmarshal(readArtifact(t, filepath.Dir(path), filepath.Base(path)), &decoded); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		for _, version := range collectProtocolVersions(decoded) {
			stated++
			if version != protocol.Version {
				t.Errorf("canonical sample %s states protocol version %q, which this build does not serve",
					filepath.Base(path), version)
			}
		}
	}
	if stated == 0 {
		t.Error("no canonical sample states a protocol version; the handshake fixtures stopped covering it")
	}
}

var protocolVersionKeys = []string{"protocolVersion"}

func collectProtocolVersions(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var out []string
		for key, value := range typed {
			if version, ok := value.(string); ok && slices.Contains(protocolVersionKeys, key) {
				out = append(out, version)
				continue
			}
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	case []any:
		var out []string
		for _, value := range typed {
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	default:
		return nil
	}
}
