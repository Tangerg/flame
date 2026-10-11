package bootstrap

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func TestProductionIdentityFactoriesProduceNamespacedFreshValues(t *testing.T) {
	t.Parallel()

	firstSession, secondSession := newSessionID(), newSessionID()
	firstRun, secondRun := newRunID(), newRunID()
	for name, check := range map[string]bool{
		"session prefix": strings.HasPrefix(firstSession, session.IDPrefix),
		"run prefix":     strings.HasPrefix(firstRun, runs.NewRunID("")),
		"session fresh":  firstSession != secondSession,
		"run fresh":      firstRun != secondRun,
	} {
		if !check {
			t.Errorf("identity invariant failed: %s", name)
		}
	}
}
