package mcpconnection

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testInstallationSource(t *testing.T) mcpserver.Source {
	t.Helper()
	source, err := mcpserver.InstallationSource(testsupport.InstallationID(t), testsupport.Digest("release"), testsupport.Digest("approved authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	return source
}
