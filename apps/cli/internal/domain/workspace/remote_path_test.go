package workspace

import "testing"

func TestRuntimeReferencesRemainIndependentOfClientFilesystem(t *testing.T) {
	for _, path := range []string{`C:\work\project`, `\\host\share\project`, "/server/project", "relative-input"} {
		request := ResolveRequest{Path: path}
		if err := request.Validate(); err != nil {
			t.Fatalf("Runtime reference %q was interpreted locally: %v", path, err)
		}
		if request.Path != path {
			t.Fatalf("Runtime reference changed: %q", request.Path)
		}
	}
}
