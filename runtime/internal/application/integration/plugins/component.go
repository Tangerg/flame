package plugins

import (
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
)

func ComponentID(unit framehost.UnitID) (plugin.ContributionID, error) {
	id, found := strings.CutPrefix(unit.String(), componentUnitPrefix)
	if !found {
		return "", plugin.ErrInvalid
	}
	return plugin.ParseContributionID(id)
}
