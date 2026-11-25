package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

type azureResource struct {
	origin httporigin.Origin
	mount  string
}

type azureDeployments map[azureResource][]string

const azureDeploymentsVariable = "AZURE_DEPLOYMENTS"

func parseAzureResource(raw string) (azureResource, error) {
	endpoint, err := httporigin.ParseEndpoint(raw)
	if err != nil || endpoint.HasQuery() || endpoint.HasFragment() {
		return azureResource{}, errors.New("model provider: invalid Azure deployment endpoint")
	}
	mount := strings.TrimRight(endpoint.URL().EscapedPath(), "/")
	if !strings.HasSuffix(mount, azureAPIPath) {
		return azureResource{}, fmt.Errorf("model provider: Azure deployment endpoint must end in %s", azureAPIPath)
	}
	return azureResource{origin: endpoint.Origin(), mount: mount}, nil
}

func parseAzureDeployments(raw string) (azureDeployments, error) {
	if raw == "" {
		return nil, nil
	}
	var configured map[string][]string
	if err := json.Unmarshal([]byte(raw), &configured); err != nil || configured == nil {
		return nil, errors.New("model provider: Azure deployments must be an endpoint-to-names object")
	}
	deployments := make(azureDeployments, len(configured))
	for endpoint, names := range configured {
		resource, err := parseAzureResource(endpoint)
		if err != nil {
			return nil, err
		}
		if _, exists := deployments[resource]; exists {
			return nil, errors.New("model provider: repeated Azure deployment resource")
		}
		if names == nil {
			return nil, errors.New("model provider: Azure deployment names must be an array")
		}
		slices.Sort(names)
		for index, name := range names {
			if _, err := flamemodel.NewModelIdentity(name); err != nil || index > 0 && names[index-1] == name {
				return nil, errors.New("model provider: invalid or repeated Azure deployment name")
			}
		}
		deployments[resource] = names
	}
	return deployments, nil
}

func (d azureDeployments) forEndpoint(endpoint string) ([]string, error) {
	resource, err := parseAzureResource(endpoint)
	if err != nil {
		return nil, err
	}
	return append([]string{}, d[resource]...), nil
}
