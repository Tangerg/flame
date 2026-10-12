package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// AutomationAdmission reads installation authority inside the Run opening
// transaction. A concurrent disable commits either before acceptance or after it.
type AutomationAdmission struct{ store Store }

func NewAutomationAdmission(store Store) (*AutomationAdmission, error) {
	if dependency.Missing(store) {
		return nil, errors.New("plugins: automation installation store is required")
	}
	return &AutomationAdmission{store: store}, nil
}
func (a *AutomationAdmission) Admit(ctx context.Context, request runs.AutomationStart) error {
	id, err := resourceid.ParseInstallation(request.Origin.InstallationID)
	if err != nil {
		return err
	}
	snapshot, found, err := a.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found || !snapshot.Installation.Enabled() || snapshot.Selected.Digest().String() != request.Origin.ReleaseDigest {
		return fmt.Errorf("%w: automated Run origin was withdrawn", plugin.ErrUnavailable)
	}
	service, found := snapshot.Selected.Service(plugin.SchedulesService)
	if !found || service.Server.String() != request.Origin.ServerName || !snapshot.Installation.ServerEnabled(service.Server) {
		return fmt.Errorf("%w: scheduling service was withdrawn", plugin.ErrUnavailable)
	}
	return nil
}
