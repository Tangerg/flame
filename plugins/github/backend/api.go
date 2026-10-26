package main

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

const (
	apiOrigin        = "https://api.github.com"
	apiVersion       = "2026-03-10"
	maxResponseBytes = 4 << 20
	maxFileBytes     = 1 << 20
)

type connector struct {
	cache      *cache
	http       *http.Client
	credential credential
}

func newConnector(cache *cache, transport http.RoundTripper, credential credential) (*connector, error) {
	if cache == nil || cache.db == nil || transport == nil {
		return nil, errors.New("github: cache and HTTP transport are required")
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &connector{cache: cache, http: client, credential: credential}, nil
}

func refused(code, message string) error {
	return &extension.Refusal{Code: code, Message: "github: " + message}
}

func query[T any](ctx context.Context, c *connector, path string, parameters url.Values, decode func(response) (T, error)) (T, error) {
	var zero T
	address := apiOrigin + path
	if len(parameters) != 0 {
		address += "?" + parameters.Encode()
	}
	key := c.credential.cacheKey(apiVersion + "\n" + address)
	result, err := c.load(ctx, address, key)
	if err != nil {
		return zero, err
	}
	value, err := decode(result)
	if err != nil {
		return zero, err
	}
	if err := c.cache.save(ctx, key, result); err != nil {
		return zero, err
	}
	return value, nil
}

func (c *connector) load(ctx context.Context, address, key string) (result response, err error) {
	previous, err := c.cache.read(ctx, key)
	if err != nil {
		return response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return response{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "Flame-GitHub-Plugin/1.0")
	if c.credential.configured() {
		request.Header.Set("authorization", c.credential.authorization())
	}
	if previous.etag != "" {
		request.Header.Set("If-None-Match", previous.etag)
	}
	upstream, err := c.http.Do(request)
	if err != nil {
		return response{}, fmt.Errorf("github: request: %w", err)
	}
	defer func() { err = errors.Join(err, upstream.Body.Close()) }()
	switch upstream.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(upstream.Body, maxResponseBytes+1))
		if err != nil {
			return response{}, err
		}
		if len(body) > maxResponseBytes {
			return response{}, refused("response_too_large", "response exceeds four MiB")
		}
		return response{body: body, etag: upstream.Header.Get("ETag"), links: upstream.Header.Get("Link")}, nil
	case http.StatusNotModified:
		if previous.etag == "" {
			return response{}, errors.New("github: conditional response has no original representation")
		}
		if tag := upstream.Header.Get("ETag"); tag != "" {
			previous.etag = tag
		}
		if links := upstream.Header.Get("Link"); links != "" {
			previous.links = links
		}
		return previous, nil
	case http.StatusUnauthorized:
		return response{}, refused("unauthorized", "credential was rejected")
	case http.StatusForbidden, http.StatusTooManyRequests:
		if upstream.StatusCode == http.StatusTooManyRequests || upstream.Header.Get("X-RateLimit-Remaining") == "0" || upstream.Header.Get("Retry-After") != "" {
			return response{}, refused("rate_limited", "request is rate limited; no retry was attempted")
		}
		return response{}, refused("forbidden", "repository access was refused")
	case http.StatusNotFound:
		return response{}, refused("not_found", "resource is absent or not visible to this credential")
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return response{}, refused("invalid_query", "request was rejected")
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return response{}, refused("moved_resource", "resource moved; supply its current repository explicitly")
	default:
		return response{}, fmt.Errorf("github: unexpected HTTP status %d", upstream.StatusCode)
	}
}

type issueWire struct {
	Number int64     `json:"number"`
	Title  string    `json:"title"`
	Body   *string   `json:"body"`
	State  rpc.State `json:"state"`
	URL    string    `json:"html_url"`
	User   *struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct{} `json:"pull_request"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (wire issueWire) admit(repository repository) (rpc.Issue, error) {
	if wire.Number <= 0 || wire.Title == "" || wire.URL == "" || wire.UpdatedAt.IsZero() || wire.State != rpc.Open && wire.State != rpc.Closed {
		return rpc.Issue{}, errors.New("github: invalid issue representation")
	}
	result := rpc.Issue{Repository: repository.String(), Number: wire.Number, Title: wire.Title, State: wire.State, URL: wire.URL, PullRequest: wire.PullRequest != nil, UpdatedAt: wire.UpdatedAt}
	if wire.Body != nil {
		result.Body = *wire.Body
	}
	if wire.User != nil {
		result.Author = wire.User.Login
	}
	return result, nil
}

func (c *connector) listIssues(ctx context.Context, input rpc.ListInput) (rpc.IssuePage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.IssuePage{}, err
	}
	parameters := url.Values{"state": {string(input.State)}, "page": {strconv.FormatInt(input.Page, 10)}, "per_page": {strconv.Itoa(input.Limit)}, "sort": {"updated"}, "direction": {"desc"}}
	path := repository.apiPath() + "/issues"
	return query(ctx, c, path, parameters, func(result response) (rpc.IssuePage, error) {
		var wire []issueWire
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return rpc.IssuePage{}, err
		}
		if wire == nil {
			return rpc.IssuePage{}, errors.New("github: issue page must be an array")
		}
		if len(wire) > input.Limit {
			return rpc.IssuePage{}, errors.New("github: issue page exceeds the requested limit")
		}
		page := rpc.IssuePage{Issues: make([]rpc.Issue, len(wire))}
		for i, row := range wire {
			var err error
			page.Issues[i], err = row.admit(repository)
			if err != nil {
				return rpc.IssuePage{}, err
			}
		}
		next, err := nextPage(result.links, input.Page)
		if err != nil {
			return rpc.IssuePage{}, err
		}
		page.NextPage = next
		return page, nil
	})
}

func nextPage(header string, current int64) (*int64, error) {
	for _, link := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) < 2 {
			continue
		}
		next := false
		for _, parameter := range parts[1:] {
			if strings.TrimSpace(parameter) == `rel="next"` {
				next = true
			}
		}
		if !next {
			continue
		}
		if !strings.HasPrefix(parts[0], "<") || !strings.HasSuffix(parts[0], ">") {
			return nil, errors.New("github: invalid pagination link")
		}
		target, err := url.Parse(strings.TrimSuffix(strings.TrimPrefix(parts[0], "<"), ">"))
		if err != nil || target.User != nil || target.Fragment != "" || target.Scheme+"://"+target.Host != apiOrigin {
			return nil, errors.New("github: invalid pagination origin")
		}
		// Only the page number is projected; later requests retain the caller's repository.
		page, err := strconv.ParseInt(target.Query().Get("page"), 10, 64)
		if err != nil || page <= current {
			return nil, errors.New("github: invalid next page")
		}
		return &page, nil
	}
	return nil, nil
}

func (c *connector) getIssue(ctx context.Context, input rpc.IssueInput) (rpc.Issue, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.Issue{}, err
	}
	return query(ctx, c, repository.apiPath()+"/issues/"+strconv.FormatInt(input.Number, 10), nil, func(result response) (rpc.Issue, error) {
		var wire issueWire
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return rpc.Issue{}, err
		}
		if wire.Number != input.Number {
			return rpc.Issue{}, errors.New("github: returned issue number differs")
		}
		return wire.admit(repository)
	})
}

func (c *connector) readFile(ctx context.Context, input rpc.FileInput) (rpc.File, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.File{}, err
	}
	path, err := newFilePath(input.Path)
	if err != nil {
		return rpc.File{}, err
	}
	parameters := url.Values{}
	if input.Ref != nil {
		parameters.Set("ref", *input.Ref)
	}
	return query(ctx, c, repository.apiPath()+"/contents/"+path.apiPath(), parameters, func(result response) (rpc.File, error) {
		var wire struct {
			Type     string `json:"type"`
			Path     string `json:"path"`
			SHA      string `json:"sha"`
			URL      string `json:"html_url"`
			Size     *int64 `json:"size"`
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		}
		if strings.HasPrefix(strings.TrimSpace(string(result.body)), "[") {
			return rpc.File{}, refused("not_file", "resource is a directory")
		}
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return rpc.File{}, err
		}
		if wire.Type == "" {
			return rpc.File{}, errors.New("github: incomplete file representation")
		}
		if wire.Type != "file" {
			return rpc.File{}, refused("not_file", "resource is not a regular file")
		}
		if wire.Encoding == "" || wire.Size == nil || *wire.Size < 0 {
			return rpc.File{}, errors.New("github: incomplete file representation")
		}
		if *wire.Size > maxFileBytes || wire.Encoding != "base64" {
			return rpc.File{}, refused("unsupported_file", "file must be base64 content up to one MiB")
		}
		content, err := base64.StdEncoding.DecodeString(wire.Content)
		if err != nil {
			return rpc.File{}, err
		}
		if int64(len(content)) != *wire.Size || wire.Path != input.Path || wire.SHA == "" || wire.URL == "" {
			return rpc.File{}, errors.New("github: inconsistent file representation")
		}
		if !utf8.Valid(content) {
			return rpc.File{}, refused("binary_file", "file content is not UTF-8")
		}
		return rpc.File{Repository: repository.String(), Path: wire.Path, SHA: wire.SHA, URL: wire.URL, Text: string(content)}, nil
	})
}
