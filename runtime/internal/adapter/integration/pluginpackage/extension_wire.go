package pluginpackage

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"

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

type wireRequestGrant struct {
	Capability string   `json:"capability"`
	Targets    []string `json:"targets"`
}
type wireInput struct {
	ID       string `json:"id"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Server   string `json:"server"`
	Target   string `json:"target"`
	Key      string `json:"key,omitempty"`
}
type wireTheme struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Scheme string            `json:"scheme"`
	Colors map[string]string `json:"colors"`
}

func (v wireRequestGrant) domain() plugin.RequestGrant {
	result := plugin.RequestGrant{Capability: plugin.Capability(v.Capability), Targets: v.Targets}
	return result
}
func (v wireInput) domain() plugin.Input {
	result := plugin.Input{ID: v.ID, Secret: v.Secret, Required: v.Required, Server: v.Server, Target: plugin.InputTarget(v.Target), Key: v.Key}
	return result
}
func (v wireTheme) domain() plugin.Theme {
	result := plugin.Theme{ID: v.ID, Title: v.Title, Scheme: plugin.ThemeScheme(v.Scheme), Colors: v.Colors}
	return result
}

type wireExtension struct {
	APIVersion  int            `json:"apiVersion"`
	Requests    jsontext.Value `json:"requests"`
	Inputs      jsontext.Value `json:"inputs"`
	Contributes jsontext.Value `json:"contributes"`
}
