package pluginpackage

import (
	"encoding/json/jsontext"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type wireContribution struct {
	ID       plugin.ContributionID `json:"id"`
	Server   mcpserver.ServerName  `json:"server"`
	Export   string                `json:"export"`
	Contract string                `json:"contract"`
	Requires []wireRequirement     `json:"requires,omitzero"`
}

type wireRequirement struct {
	ID       plugin.ContributionID `json:"id"`
	Contract string                `json:"contract"`
	Optional bool                  `json:"optional"`
}

type wirePoint struct {
	ID       plugin.ContributionID `json:"id"`
	Contract string                `json:"contract"`
}

type wirePointValue struct {
	ID       plugin.ContributionID `json:"id"`
	Contract string                `json:"contract"`
	Value    jsontext.Value        `json:"value"`
}

type wireComponent struct {
	ID           plugin.ContributionID  `json:"id"`
	Title        string                 `json:"title"`
	Scope        string                 `json:"scope"`
	Placement    string                 `json:"placement"`
	Entry        string                 `json:"entry"`
	Export       string                 `json:"export"`
	Contribution *plugin.ContributionID `json:"contribution,omitzero"`
	Material     *string                `json:"material,omitzero"`
}

type wireInput struct {
	ID       string               `json:"id"`
	Secret   bool                 `json:"secret"`
	Required bool                 `json:"required"`
	Server   mcpserver.ServerName `json:"server"`
	Target   string               `json:"target"`
	Key      string               `json:"key,omitempty"`
}
type wireTheme struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Scheme string          `json:"scheme"`
	Colors wireThemeColors `json:"colors"`
}
type wireView struct {
	ID                string                 `json:"id"`
	Title             string                 `json:"title"`
	Type              plugin.ViewKind        `json:"type"`
	Entry             string                 `json:"entry"`
	ScheduleTemplates []wireScheduleTemplate `json:"scheduleTemplates,omitempty"`
}
type wireScheduleTemplate struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Instructions string `json:"instructions"`
	Cron         string `json:"cron"`
}

type wireAction struct {
	ID        string                 `json:"id"`
	Title     string                 `json:"title"`
	Operation plugin.ActionOperation `json:"operation"`
}
type wireRenderer struct {
	ID       string               `json:"id"`
	Title    string               `json:"title"`
	Selector wireRendererSelector `json:"selector"`
	Entry    string               `json:"entry"`
}
type wireRendererSelector struct {
	Type string `json:"type"`
	Name string `json:"name"`
}
type wireThemeColors struct {
	Background *string `json:"background,omitempty"`
	Foreground *string `json:"foreground,omitempty"`
	Accent     *string `json:"accent,omitempty"`
	Muted      *string `json:"muted,omitempty"`
	Border     *string `json:"border,omitempty"`
}

func (v wireInput) domain() plugin.Input {
	return plugin.Input{ID: v.ID, Secret: v.Secret, Required: v.Required, Server: v.Server, Target: plugin.InputTarget(v.Target), Key: v.Key}
}

// domain refuses an authored empty color: an absent member is the only
// spelling of a color the theme does not override.
func (v wireTheme) domain() (plugin.Theme, error) {
	var colors plugin.ThemeColors
	for _, member := range []struct {
		authored *string
		target   *string
	}{
		{v.Colors.Background, &colors.Background},
		{v.Colors.Foreground, &colors.Foreground},
		{v.Colors.Accent, &colors.Accent},
		{v.Colors.Muted, &colors.Muted},
		{v.Colors.Border, &colors.Border},
	} {
		if member.authored == nil {
			continue
		}
		if *member.authored == "" {
			return plugin.Theme{}, errors.New("pluginpackage: theme color is empty")
		}
		*member.target = *member.authored
	}
	return plugin.Theme{ID: v.ID, Title: v.Title, Scheme: plugin.ThemeScheme(v.Scheme), Colors: colors}, nil
}

var declarationFields = []string{"protocol", "inputs", "contributions", "contributes", "points", "pointValues", "components"}
var presentationFields = []string{"views", "renderers", "actions", "themes"}
