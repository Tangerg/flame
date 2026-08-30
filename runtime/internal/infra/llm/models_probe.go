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
)

// remoteModelList is the OpenAI GET /v1/models response shape, which Ollama /
// LM Studio / vLLM / OpenRouter and Anthropic's list endpoint all emit. Only the
// ids matter here; capability/pricing are enriched from the static catalog by
// the caller when the id is known.
type remoteModelList struct {
	Data []remoteModel `json:"data"`
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

// ListRemoteModels probes a compatible provider endpoint for its models
// (GET {baseURL}/models) and returns the advertised model ids, sorted and
// de-duplicated. It backs live model discovery for local / bring-your-own-
// endpoint providers whose model set is user-defined rather than in the static
// catalog (Ollama and the compatible endpoint providers). apiKey rides as a bearer token when
// non-empty (a local daemon needs none). The call is bounded (timeout + response
// cap); a non-200 or unparseable body is returned as an error the caller treats
// as "no discovery" and falls back to the static catalog.
func ListRemoteModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

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
	ids, err := document.identities()
	if err != nil {
		return nil, fmt.Errorf("llm: model probe %s: %w", endpoint, err)
	}
	return ids, nil
}
