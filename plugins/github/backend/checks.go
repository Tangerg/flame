package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"strconv"
	"time"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
)

type checkWire struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	HeadSHA     string     `json:"head_sha"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	URL         string     `json:"html_url"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Output      *struct {
		Title            *string `json:"title"`
		Summary          *string `json:"summary"`
		Text             *string `json:"text"`
		AnnotationsCount *int64  `json:"annotations_count"`
	} `json:"output"`
}

func (wire checkWire) admit() (contractwire.CheckRun, error) {
	if wire.ID <= 0 || wire.Name == "" || wire.HeadSHA == "" || wire.Status == "" || wire.Output == nil || wire.Output.AnnotationsCount == nil || *wire.Output.AnnotationsCount < 0 {
		return contractwire.CheckRun{}, errors.New("github: incomplete check run representation")
	}
	result := contractwire.CheckRun{ID: wire.ID, Name: wire.Name, HeadSHA: wire.HeadSHA, Status: wire.Status, Conclusion: wire.Conclusion, URL: wire.URL, StartedAt: wire.StartedAt, CompletedAt: wire.CompletedAt, AnnotationsCount: *wire.Output.AnnotationsCount}
	if wire.Output.Title != nil {
		result.Title = *wire.Output.Title
	}
	if wire.Output.Summary != nil {
		result.Summary = *wire.Output.Summary
	}
	if wire.Output.Text != nil {
		result.Text = *wire.Output.Text
	}
	return result, nil
}

func (c *connector) listChecks(ctx context.Context, input contractwire.CommitInput) (contractwire.CheckPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.CheckPage{}, err
	}
	parameters := pagination(input.Page, input.Limit)
	parameters.Set("filter", "latest")
	path := repository.apiPath() + "/commits/" + url.PathEscape(input.Ref) + "/check-runs"
	return query(ctx, c, path, parameters, func(result response) (contractwire.CheckPage, error) {
		var wire struct {
			TotalCount *int64      `json:"total_count"`
			CheckRuns  []checkWire `json:"check_runs"`
		}
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return contractwire.CheckPage{}, err
		}
		if wire.TotalCount == nil || *wire.TotalCount < int64(len(wire.CheckRuns)) {
			return contractwire.CheckPage{}, errors.New("github: inconsistent check run count")
		}
		rows, err := admitRows(wire.CheckRuns, input.Limit, checkWire.admit)
		if err != nil {
			return contractwire.CheckPage{}, err
		}
		next, err := nextPage(result.links, input.Page)
		return contractwire.CheckPage{CheckRuns: rows, NextPage: next}, err
	})
}

type annotationWire struct {
	Path        string  `json:"path"`
	StartLine   int64   `json:"start_line"`
	EndLine     int64   `json:"end_line"`
	StartColumn *int64  `json:"start_column"`
	EndColumn   *int64  `json:"end_column"`
	Level       string  `json:"annotation_level"`
	Message     *string `json:"message"`
	Title       *string `json:"title"`
	RawDetails  *string `json:"raw_details"`
}

func (wire annotationWire) admit() (contractwire.Annotation, error) {
	if wire.Path == "" || wire.StartLine < 1 || wire.EndLine < wire.StartLine || wire.Level == "" || wire.Message == nil {
		return contractwire.Annotation{}, errors.New("github: incomplete check annotation")
	}
	if wire.StartColumn != nil && *wire.StartColumn < 1 || wire.EndColumn != nil && *wire.EndColumn < 1 {
		return contractwire.Annotation{}, errors.New("github: invalid check annotation column")
	}
	return contractwire.Annotation{Path: wire.Path, StartLine: wire.StartLine, EndLine: wire.EndLine, StartColumn: wire.StartColumn, EndColumn: wire.EndColumn, Level: wire.Level, Message: *wire.Message, Title: wire.Title, RawDetails: wire.RawDetails}, nil
}

func (c *connector) listAnnotations(ctx context.Context, input contractwire.CheckInput) (contractwire.AnnotationPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.AnnotationPage{}, err
	}
	path := repository.apiPath() + "/check-runs/" + strconv.FormatInt(input.CheckRunID, 10) + "/annotations"
	page, err := readPage(ctx, c, path, pagination(input.Page, input.Limit), input.Page, input.Limit, annotationWire.admit)
	return contractwire.AnnotationPage{Annotations: page.rows, NextPage: page.next}, err
}

type statusWire struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"`
	Context     string    `json:"context"`
	Description *string   `json:"description"`
	URL         *string   `json:"target_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (wire statusWire) admit() (contractwire.CommitStatus, error) {
	if wire.ID <= 0 || wire.State == "" || wire.Context == "" || wire.CreatedAt.IsZero() || wire.UpdatedAt.IsZero() {
		return contractwire.CommitStatus{}, errors.New("github: incomplete commit status representation")
	}
	return contractwire.CommitStatus{ID: wire.ID, State: wire.State, Context: wire.Context, Description: wire.Description, URL: wire.URL, CreatedAt: wire.CreatedAt, UpdatedAt: wire.UpdatedAt}, nil
}

func (c *connector) listStatuses(ctx context.Context, input contractwire.CommitInput) (contractwire.StatusPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.StatusPage{}, err
	}
	path := repository.apiPath() + "/commits/" + url.PathEscape(input.Ref) + "/statuses"
	page, err := readPage(ctx, c, path, pagination(input.Page, input.Limit), input.Page, input.Limit, statusWire.admit)
	return contractwire.StatusPage{Statuses: page.rows, NextPage: page.next}, err
}
