package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type pendingInstallationCheckpoints [][]byte

func (p pendingInstallationCheckpoints) PendingCheckpointPayloads(context.Context) ([][]byte, error) {
	return p, nil
}

func TestInstallationChangeReadsOnlyCheckpointDependencies(t *testing.T) {
	id := "940ac827-b431-455b-af4b-e3a170bcfda0"
	for _, test := range []struct {
		name    string
		payload string
		refused bool
	}{
		{name: "unrelated broken continuation", payload: `{"installations":[],"toolBindings":[],"tree":null,"options":"broken"}`},
		{name: "declared dependency", payload: `{"installations":[{"installationId":"` + id + `","digest":"` + strings.Repeat("1", 64) + `"}],"toolBindings":[],"tree":null}`, refused: true},
		{name: "missing dependencies", payload: `{"tree":null}`, refused: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("installation change invoked model")
			}), InteractionExecutorConfig{InstallationCheckpoints: pendingInstallationCheckpoints{[]byte(test.payload)}})
			changed := false
			err := executor.ChangeInstallation(t.Context(), id, plugins.RequireQuiescent, func() error {
				changed = true
				return nil
			})
			if test.refused {
				if err == nil || changed {
					t.Fatalf("unsafe change admitted: %v, changed=%v", err, changed)
				}
				if test.name == "declared dependency" && !errors.Is(err, plugin.ErrInUse) {
					t.Fatal(err)
				}
			} else if err != nil || !changed {
				t.Fatalf("unrelated continuation prevented change: %v, changed=%v", err, changed)
			}
		})
	}
}
