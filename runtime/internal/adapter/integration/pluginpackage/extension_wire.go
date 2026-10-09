package pluginpackage

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func decodeDeclaration(raw jsontext.Value, target any) error {
	decoder := jsontext.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if token.Kind() == 'n' {
			return errors.New("pluginpackage: null is not a declaration value")
		}
	}
	return json.Unmarshal(raw, target, json.RejectUnknownMembers(true))
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
	ID    string          `json:"id"`
	Title string          `json:"title"`
	Type  plugin.ViewKind `json:"type"`
	Entry string          `json:"entry"`
}
type wireAction struct {
	ID        string                 `json:"id"`
	Title     string                 `json:"title"`
	Operation plugin.ActionOperation `json:"operation"`
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

// extensionFields are the members of the Flame namespace this API version
// supports. Any other member is reported as an unknown field and ignored, so
// a declaration Flame no longer supports, such as capability requests, is
// visible to the user and never silently accepted or enforced.
var extensionFields = []string{"apiVersion", "inputs", "contributes"}
