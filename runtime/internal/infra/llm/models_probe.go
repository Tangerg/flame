package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

const (
	probeTimeout                   = 4 * time.Second
	maximumModelProbeResponseBytes = 1 << 20
	maximumRemoteModelCount        = 4096
	anthropicProtocolVersion       = "2023-06-01"
	anthropicMaximumModelPageSize  = 1000
)

// remoteModelList is the OpenAI GET /v1/models response shape, which Ollama /
// LM Studio / vLLM / OpenRouter and Anthropic's list endpoint all emit. Only the
// ids matter here; capability/pricing are enriched from the static catalog by
// the caller when the id is known.
type remoteModelList struct {
	Data    []remoteModel `json:"data"`
	HasMore bool          `json:"has_more"`
}

type remoteModel struct {
	ID string `json:"id"`
}

// modelProbeDocument is one complete, bounded endpoint response. Admission is
// decided before any advertised identity can escape into the model catalog.
type modelProbeDocument struct {
	list remoteModelList
}

func readModelProbeDocument(source io.Reader) (modelProbeDocument, error) {
	content, err := io.ReadAll(io.LimitReader(source, maximumModelProbeResponseBytes+1))
	if err != nil {
		return modelProbeDocument{}, fmt.Errorf("read response: %w", err)
	}
	if len(content) > maximumModelProbeResponseBytes {
		return modelProbeDocument{}, fmt.Errorf("response exceeds %d bytes", maximumModelProbeResponseBytes)
	}
	if !utf8.Valid(content) {
		return modelProbeDocument{}, errors.New("response is not valid UTF-8")
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	var list remoteModelList
	if err := decoder.Decode(&list); err != nil {
		return modelProbeDocument{}, fmt.Errorf("decode response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return modelProbeDocument{}, errors.New("response contains a trailing JSON value")
		}
		return modelProbeDocument{}, fmt.Errorf("decode trailing response material: %w", err)
	}
	if len(list.Data) > maximumRemoteModelCount {
		return modelProbeDocument{}, fmt.Errorf(
			"response advertises %d models; maximum is %d",
			len(list.Data), maximumRemoteModelCount,
		)
	}
	return modelProbeDocument{list: list}, nil
}

func (d modelProbeDocument) identities() ([]string, error) {
	ids := make([]string, 0, len(d.list.Data))
	for _, model := range d.list.Data {
		identity, err := modelref.NewModelIdentity(model.ID)
		if err != nil {
			return nil, fmt.Errorf("advertised model identity %q: %w", model.ID, err)
		}
		ids = append(ids, identity.String())
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// listRemoteModels probes a compatible provider endpoint with the listing
// protocol owned by its provider profile. OpenAI-family endpoints expose
// {baseURL}/models with bearer authentication; Anthropic-family endpoints
// expose {baseURL}/v1/models with native headers. The response remains bounded
// regardless of protocol.
func listRemoteModels(ctx context.Context, baseURL, apiKey string, protocol modelListProtocol) ([]string, error) {
	endpoint, err := protocol.modelListEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	protocol.authorize(req, apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: model probe %s: status %d", endpoint, resp.StatusCode)
	}

	document, err := readModelProbeDocument(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: model probe %s: %w", endpoint, err)
	}
	if protocol == modelListProtocolAnthropic && document.list.HasMore {
		return nil, fmt.Errorf(
			"llm: model probe %s: catalog exceeds the maximum %d-model page",
			endpoint,
			anthropicMaximumModelPageSize,
		)
	}
	ids, err := document.identities()
	if err != nil {
		return nil, fmt.Errorf("llm: model probe %s: %w", endpoint, err)
	}
	return ids, nil
}

func (p modelListProtocol) modelListEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	switch p {
	case modelListProtocolOpenAI:
		return baseURL + "/models", nil
	case modelListProtocolAnthropic:
		return fmt.Sprintf("%s/v1/models?limit=%d", baseURL, anthropicMaximumModelPageSize), nil
	default:
		return "", fmt.Errorf("llm: unsupported model listing protocol %d", p)
	}
}

func (p modelListProtocol) authorize(request *http.Request, apiKey string) {
	switch p {
	case modelListProtocolOpenAI:
		if apiKey != "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	case modelListProtocolAnthropic:
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", anthropicProtocolVersion)
	}
}
