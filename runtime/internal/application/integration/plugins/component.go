package plugins

import (
	"strings"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func ComponentID(unit qimu.UnitID) (plugin.ContributionID, error) {
	id, found := strings.CutPrefix(unit.String(), "component/")
	if !found {
		return "", plugin.ErrInvalid
	}
	return plugin.ParseContributionID(id)
}
