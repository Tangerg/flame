package main

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

// Azure's inference API accepts deployment names, while /models publishes the
// resource's available models. A deployment list belongs to one exact endpoint.
type azureDeployments map[string][]string

const azureDeploymentsVariable = "AZURE_DEPLOYMENTS"

func parseAzureDeployments(raw string) (azureDeployments, error) {
	deployments := make(azureDeployments)
	if raw == "" {
		return deployments, nil
	}
	if err := json.Unmarshal([]byte(raw), &deployments); err != nil || deployments == nil {
		return nil, errors.New("model provider: Azure deployments must be an endpoint-to-names object")
	}
	for endpoint, names := range deployments {
		if azureMount.ValidateOverride(endpoint) != nil || endpoint == "" || strings.HasSuffix(endpoint, "/") {
			return nil, errors.New("model provider: expected an Azure deployment endpoint ending in /openai/v1")
		}
		if names == nil {
			return nil, errors.New("model provider: Azure deployment names must be an array")
		}
		slices.Sort(names)
		for index, name := range names {
			if _, err := modelchoice.NewModelIdentity(name); err != nil || index > 0 && names[index-1] == name {
				return nil, errors.New("model provider: invalid or repeated Azure deployment name")
			}
		}
	}
	return deployments, nil
}

func (d azureDeployments) forEndpoint(endpoint string) []string {
	return append([]string{}, d[strings.TrimSuffix(endpoint, "/")]...)
}
