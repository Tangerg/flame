package plugins

import (
	"context"

	framehost "github.com/Tangerg/frame/host"
)

// Native slots survive release changes. Join the original instance outside
// admission before acquiring its successor; Frame alone owns the release proof.
func AwaitNativeUnitRetirement(ctx context.Context, host framehost.Host, unit framehost.UnitRef) error {
	for _, observed := range host.Observations() {
		if observed.State == framehost.Preparing || observed.State == framehost.Ready {
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
