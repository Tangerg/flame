package bootstrap

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func mustBootstrapRole(t testing.TB, provider, model string) modelref.Role {
	t.Helper()
	role, err := modelref.NewRole(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}
