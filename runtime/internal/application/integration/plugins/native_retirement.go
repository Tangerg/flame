package plugins

import (
	"context"

	"github.com/Tangerg/frame"
)

// Native slots survive release changes. Join the original instance outside
// admission before acquiring its successor; Frame alone owns the release proof.
func AwaitNativeUnitRetirement(ctx context.Context, host *frame.Host, unit frame.UnitRef) error {
	for _, observed := range host.Observations() {
		if observed.State == frame.Preparing || observed.State == frame.Ready {
			continue
		}
		if observed.Source.InstallationID != unit.InstallationID || observed.Source.UnitID != unit.UnitID {
			continue
		}
		if err := host.WaitInstanceRetired(ctx, observed.ID); err != nil {
			return err
		}
	}
	return nil
}
