package main

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"

	framecall "github.com/Tangerg/frame/call"
)

const (
	apiOrigin        = "https://api.github.com"
	apiVersion       = "2026-03-10"
	maxResponseBytes = 4 << 20
	maxFileBytes     = 1 << 20
)

type connector struct {
	store      *store
	http       *http.Client
	credential credential
}

const requestTimeout = 30 * time.Second

func newConnector(store *store, transport http.RoundTripper, credential credential) (*connector, error) {
	if store == nil || !store.db.Valid() || transport == nil {
		return nil, errors.New("github: store and HTTP transport are required")
	}
	client := &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &connector{store: store, http: client, credential: credential}, nil
}

// GitHub refusal codes name why a request was refused; callers do not retry them.
const (
	refusalBinaryFile         = "binary_file"
	refusalConflict           = "conflict"
	refusalCredentialRequired = "credential_required"
	refusalForbidden          = "forbidden"
	refusalInvalidQuery       = "invalid_query"
	refusalInvalidRequest     = "invalid_request"
	refusalMovedResource      = "moved_resource"
	refusalNotAllowed         = "not_allowed"
	refusalNotFile            = "not_file"
	refusalNotFound           = "not_found"
	refusalRateLimited        = "rate_limited"
	refusalResponseTooLarge   = "response_too_large"
	refusalUnauthorized       = "unauthorized"
	refusalUnsupportedFile    = "unsupported_file"
)

// identify marks a request with the API version this plugin speaks and, when
// configured, the credential it acts as.
func (c *connector) identify(request *http.Request) {
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "Flame-GitHub-Plugin/1.0")
	if c.credential.configured() {
		request.Header.Set("Authorization", c.credential.authorization())
	}
}

func refused(code, message string) error {
	return &framecall.Refusal{Code: code, Message: "github: " + message}
}

func invalidInput(message string) error { return refused(pluginmcp.InvalidInputCode, message) }

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
	if err := c.store.saveCache(ctx, key, result); err != nil {
		return zero, err
	}
	return value, nil
}

func (c *connector) load(ctx context.Context, address, key string) (result response, err error) {
	previous, err := c.store.readCache(ctx, key)
	if err != nil {
		return response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return response{}, err
	}
	c.identify(request)
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
		return freshResponse(upstream)
	case http.StatusNotModified:
		return previous.revalidated(upstream.Header)
	case http.StatusUnauthorized:
		return response{}, refused(refusalUnauthorized, "credential was rejected")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return response{}, accessRefusal(upstream)
	case http.StatusNotFound:
		return response{}, refused(refusalNotFound, "resource is absent or not visible to this credential")
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return response{}, refused(refusalInvalidQuery, "request was rejected")
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return response{}, refused(refusalMovedResource, "resource moved; supply its current repository explicitly")
	default:
		return response{}, fmt.Errorf("github: unexpected HTTP status %d", upstream.StatusCode)
	}
}

func freshResponse(upstream *http.Response) (response, error) {
	body, err := io.ReadAll(io.LimitReader(upstream.Body, maxResponseBytes+1))
	if err != nil {
		return response{}, err
	}
	if len(body) > maxResponseBytes {
		return response{}, refused(refusalResponseTooLarge, "response exceeds four MiB")
	}
	return response{body: body, etag: upstream.Header.Get("ETag"), links: upstream.Header.Get("Link")}, nil
}

// revalidated is the cached representation a 304 confirmed, carrying any
// validators the confirmation refreshed.
func (r response) revalidated(header http.Header) (response, error) {
	if r.etag == "" {
		return response{}, errors.New("github: conditional response has no original representation")
	}
	if tag := header.Get("ETag"); tag != "" {
		r.etag = tag
	}
	if links := header.Get("Link"); links != "" {
		r.links = links
	}
	return r, nil
}

// GitHub can signal secondary exhaustion only in the response message, even
// while the primary budget has capacity. Do not expose arbitrary error bodies.
func accessRefusal(response *http.Response) error {
	limited := response.StatusCode == http.StatusTooManyRequests || response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != ""
	if !limited {
		const maxErrorBytes = 16 << 10
		var problem struct {
			Message string `json:"message"`
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBytes+1))
		if err != nil {
			return errors.Join(refused(refusalForbidden, "repository access was refused"), err)
		}
		limited = len(raw) <= maxErrorBytes && json.Unmarshal(raw, &problem) == nil && strings.Contains(strings.ToLower(problem.Message), "secondary rate limit")
	}
	if limited {
		return refused(refusalRateLimited, "request is rate limited; no retry was attempted")
	}
	return refused(refusalForbidden, "repository access was refused")
}

type userWire struct {
	Login string `json:"login"`
}

func author(user *userWire) string {
	if user == nil {
		return ""
	}
	return user.Login
}

func pagination(page int64, limit int) url.Values {
	return url.Values{"page": {strconv.FormatInt(page, 10)}, "per_page": {strconv.Itoa(limit)}}
}

type page[T any] struct {
	rows []T
	next *int64
}

func admitRows[W, R any](rows []W, limit int, admit func(W) (R, error)) ([]R, error) {
	if rows == nil || len(rows) > limit {
		return nil, errors.New("github: page must be an array within the requested limit")
	}
	result := make([]R, len(rows))
	for i, row := range rows {
		var err error
		result[i], err = admit(row)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// readPage reads one page of a listing; filters never carry the page cursor.
func readPage[W, R any](ctx context.Context, c *connector, path string, filters url.Values, current int64, limit int, admit func(W) (R, error)) (page[R], error) {
	parameters := pagination(current, limit)
	maps.Copy(parameters, filters)
	return query(ctx, c, path, parameters, func(result response) (page[R], error) {
		var wire []W
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return page[R]{}, err
		}
		rows, err := admitRows(wire, limit, admit)
		if err != nil {
			return page[R]{}, err
		}
		next, err := nextPage(result.links, current)
		return page[R]{rows: rows, next: next}, err
	})
}

type issueWire struct {
	ID          int64              `json:"id"`
	Number      int64              `json:"number"`
	Title       string             `json:"title"`
	Body        *string            `json:"body"`
	State       contractwire.State `json:"state"`
	URL         string             `json:"html_url"`
	User        *userWire          `json:"user"`
	PullRequest *struct{}          `json:"pull_request"`
	UpdatedAt   instant.Timestamp  `json:"updated_at"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees   []userWire `json:"assignees"`
	StateReason *string    `json:"state_reason"`
}

func (wire issueWire) admit(repository repository) (contractwire.Issue, error) {
	if wire.ID <= 0 || wire.Number <= 0 || wire.Title == "" || wire.URL == "" || wire.UpdatedAt.IsZero() || !wire.State.Valid() {
		return contractwire.Issue{}, errors.New("github: invalid issue representation")
	}
	if wire.Labels == nil || wire.Assignees == nil {
		return contractwire.Issue{}, errors.New("github: issue metadata arrays are missing")
	}
	result := contractwire.Issue{ID: wire.ID, Repository: repository.String(), Number: wire.Number, Title: wire.Title, State: wire.State, URL: wire.URL, Author: author(wire.User), PullRequest: wire.PullRequest != nil, UpdatedAt: wire.UpdatedAt}
	result.Labels, result.Assignees, result.StateReason = make([]string, len(wire.Labels)), make([]string, len(wire.Assignees)), wire.StateReason
	for i, label := range wire.Labels {
		if !textPresent(label.Name) {
			return contractwire.Issue{}, errors.New("github: issue label name is missing")
		}
		result.Labels[i] = label.Name
	}
	for i, assignee := range wire.Assignees {
		if !textPresent(assignee.Login) {
			return contractwire.Issue{}, errors.New("github: issue assignee login is missing")
		}
		result.Assignees[i] = assignee.Login
	}
	if wire.Body != nil {
		result.Body = *wire.Body
	}
	return result, nil
}

func (c *connector) listIssues(ctx context.Context, input contractwire.ListInput) (contractwire.IssuePage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.IssuePage{}, err
	}
	filters := url.Values{"state": {string(input.State)}, "sort": {"updated"}, "direction": {"desc"}}
	path := repository.apiPath() + "/issues"
	page, err := readPage(ctx, c, path, filters, input.Page, input.Limit, func(wire issueWire) (contractwire.Issue, error) { return wire.admit(repository) })
	return contractwire.IssuePage{Issues: page.rows, NextPage: page.next}, err
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

func (c *connector) getIssue(ctx context.Context, input contractwire.IssueInput) (contractwire.Issue, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.Issue{}, err
	}
	return query(ctx, c, repository.apiPath()+"/issues/"+strconv.FormatInt(input.Number, 10), nil, func(result response) (contractwire.Issue, error) {
		var wire issueWire
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return contractwire.Issue{}, err
		}
		if wire.Number != input.Number {
			return contractwire.Issue{}, errors.New("github: returned issue number differs")
		}
		return wire.admit(repository)
	})
}

func (c *connector) readFile(ctx context.Context, input contractwire.FileInput) (contractwire.File, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.File{}, err
	}
	path, err := newFilePath(input.Path)
	if err != nil {
		return contractwire.File{}, err
	}
	parameters := url.Values{}
	if input.Ref != nil {
		parameters.Set("ref", *input.Ref)
	}
	return query(ctx, c, repository.apiPath()+"/contents/"+path.apiPath(), parameters, func(result response) (contractwire.File, error) {
		if strings.HasPrefix(strings.TrimSpace(string(result.body)), "[") {
			return contractwire.File{}, refused(refusalNotFile, "resource is a directory")
		}
		var content contentResponse
		if err := json.Unmarshal(result.body, &content); err != nil {
			return contractwire.File{}, err
		}
		text, err := content.text(input.Path)
		if err != nil {
			return contractwire.File{}, err
		}
		return contractwire.File{Repository: repository.String(), Path: content.Path, SHA: content.SHA, URL: content.URL, Text: text}, nil
	})
}

// contentResponse is GitHub's contents representation of one repository path.
type contentResponse struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	URL      string `json:"html_url"`
	Size     *int64 `json:"size"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// text admits only a complete, bounded UTF-8 file at the requested path.
func (r contentResponse) text(requested string) (string, error) {
	if r.Type == "" {
		return "", errors.New("github: incomplete file representation")
	}
	if r.Type != "file" {
		return "", refused(refusalNotFile, "resource is not a regular file")
	}
	if r.Encoding == "" || r.Size == nil || *r.Size < 0 {
		return "", errors.New("github: incomplete file representation")
	}
	if *r.Size > maxFileBytes || r.Encoding != "base64" {
		return "", refused(refusalUnsupportedFile, "file must be base64 content up to one MiB")
	}
	content, err := base64.StdEncoding.DecodeString(r.Content)
	if err != nil {
		return "", err
	}
	if int64(len(content)) != *r.Size || r.Path != requested || r.SHA == "" || r.URL == "" {
		return "", errors.New("github: inconsistent file representation")
	}
	if !utf8.Valid(content) {
		return "", refused(refusalBinaryFile, "file content is not UTF-8")
	}
	return string(content), nil
}
