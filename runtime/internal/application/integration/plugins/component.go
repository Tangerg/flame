package plugins

import (
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/qimu"
)

func ComponentID(unit qimu.UnitID) (plugin.ContributionID, error) {
	id, found := strings.CutPrefix(unit.String(), componentUnitPrefix)
	if !found {
		return "", plugin.ErrInvalid
	}
	return plugin.ParseContributionID(id)
}
