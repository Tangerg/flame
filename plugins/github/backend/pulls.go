package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strconv"
	"time"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
)

type branchWire struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
	Repo  *struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}

func (wire branchWire) admit() (contractwire.Branch, error) {
	if wire.Ref == "" || wire.SHA == "" {
		return contractwire.Branch{}, errors.New("github: incomplete pull request branch")
	}
	result := contractwire.Branch{Label: wire.Label, Ref: wire.Ref, SHA: wire.SHA}
	if wire.Repo != nil {
		repository, err := newRepository(wire.Repo.FullName)
		if err != nil {
			return contractwire.Branch{}, errors.New("github: invalid pull request branch repository")
		}
		name := repository.String()
		result.Repository = &name
	}
	return result, nil
}

type pullWire struct {
	Number              int64              `json:"number"`
	Title               string             `json:"title"`
	Body                *string            `json:"body"`
	State               contractwire.State `json:"state"`
	URL                 string             `json:"html_url"`
	User                *userWire          `json:"user"`
	Draft               *bool              `json:"draft"`
	MaintainerCanModify *bool              `json:"maintainer_can_modify"`
	Head                *branchWire        `json:"head"`
	Base                *branchWire        `json:"base"`
	UpdatedAt           time.Time          `json:"updated_at"`
	MergedAt            *time.Time         `json:"merged_at"`
	Mergeable           *bool              `json:"mergeable"`
	MergeableState      *string            `json:"mergeable_state"`
	Additions           *int64             `json:"additions"`
	Deletions           *int64             `json:"deletions"`
	ChangedFiles        *int64             `json:"changed_files"`
}

func (wire pullWire) admit(repository repository) (contractwire.PullRequest, error) {
	if wire.Number <= 0 || wire.Title == "" || wire.URL == "" || wire.UpdatedAt.IsZero() || wire.State != contractwire.Open && wire.State != contractwire.Closed || wire.Draft == nil || wire.Head == nil || wire.Base == nil {
		return contractwire.PullRequest{}, errors.New("github: incomplete pull request representation")
	}
	for _, count := range []*int64{wire.Additions, wire.Deletions, wire.ChangedFiles} {
		if count != nil && *count < 0 {
			return contractwire.PullRequest{}, errors.New("github: invalid pull request change count")
		}
	}
	head, err := wire.Head.admit()
	if err != nil {
		return contractwire.PullRequest{}, err
	}
	base, err := wire.Base.admit()
	if err != nil {
		return contractwire.PullRequest{}, err
	}
	if base.Repository == nil || *base.Repository != repository.String() {
		return contractwire.PullRequest{}, errors.New("github: returned pull request repository differs")
	}
	result := contractwire.PullRequest{Repository: repository.String(), Number: wire.Number, Title: wire.Title, State: wire.State, URL: wire.URL, Author: author(wire.User), Draft: *wire.Draft, Head: head, Base: base, UpdatedAt: wire.UpdatedAt, MergedAt: wire.MergedAt, Mergeable: wire.Mergeable, MergeableState: wire.MergeableState, Additions: wire.Additions, Deletions: wire.Deletions, ChangedFiles: wire.ChangedFiles}
	result.MaintainerCanModify = wire.MaintainerCanModify
	if wire.Body != nil {
		result.Body = *wire.Body
	}
	return result, nil
}

func (c *connector) listPulls(ctx context.Context, input contractwire.PullListInput) (contractwire.PullPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.PullPage{}, err
	}
	parameters := pagination(input.Page, input.Limit)
	parameters.Set("state", string(input.State))
	parameters.Set("sort", "updated")
	parameters.Set("direction", "desc")
	if input.Base != nil {
		parameters.Set("base", *input.Base)
	}
	if input.Head != nil {
		parameters.Set("head", *input.Head)
	}
	page, err := readPage(ctx, c, repository.apiPath()+"/pulls", parameters, input.Page, input.Limit, func(wire pullWire) (contractwire.PullRequest, error) { return wire.admit(repository) })
	return contractwire.PullPage{PullRequests: page.rows, NextPage: page.next}, err
}

func (c *connector) getPull(ctx context.Context, input contractwire.IssueInput) (contractwire.PullRequest, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.PullRequest{}, err
	}
	return query(ctx, c, repository.apiPath()+"/pulls/"+strconv.FormatInt(input.Number, 10), nil, func(result response) (contractwire.PullRequest, error) {
		var wire pullWire
		if err := json.Unmarshal(result.body, &wire); err != nil {
			return contractwire.PullRequest{}, err
		}
		if wire.Number != input.Number || wire.Additions == nil || wire.Deletions == nil || wire.ChangedFiles == nil {
			return contractwire.PullRequest{}, errors.New("github: inconsistent pull request detail")
		}
		return wire.admit(repository)
	})
}

func (c *connector) listPullFiles(ctx context.Context, input contractwire.ThreadInput) (contractwire.FilePage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return contractwire.FilePage{}, err
	}
	const fileLimit = 3000
	if input.Limit < 1 || input.Page < 1 || input.Page > int64((fileLimit-1)/input.Limit+1) {
		return contractwire.FilePage{}, refused("invalid_query", "pull request files are limited to the first 3000 entries")
	}
	path := repository.apiPath() + "/pulls/" + strconv.FormatInt(input.Number, 10) + "/files"
	page, err := readPage(ctx, c, path, pagination(input.Page, input.Limit), input.Page, input.Limit, func(wire struct {
		Path         string  `json:"filename"`
		PreviousPath *string `json:"previous_filename"`
		SHA          string  `json:"sha"`
		Status       string  `json:"status"`
		Additions    *int64  `json:"additions"`
		Deletions    *int64  `json:"deletions"`
		Changes      *int64  `json:"changes"`
		Patch        *string `json:"patch"`
	}) (contractwire.ChangedFile, error) {
		if wire.Path == "" || wire.SHA == "" || wire.Status == "" || wire.Additions == nil || *wire.Additions < 0 || wire.Deletions == nil || *wire.Deletions < 0 || wire.Changes == nil || *wire.Changes < 0 {
			return contractwire.ChangedFile{}, errors.New("github: incomplete changed file representation")
		}
		return contractwire.ChangedFile{Path: wire.Path, PreviousPath: wire.PreviousPath, SHA: wire.SHA, Status: wire.Status, Additions: *wire.Additions, Deletions: *wire.Deletions, Changes: *wire.Changes, Patch: wire.Patch}, nil
	})
	result := contractwire.FilePage{Files: page.rows, NextPage: page.next, LimitReached: (input.Page-1)*int64(input.Limit)+int64(len(page.rows)) >= fileLimit}
	if result.LimitReached {
		result.NextPage = nil
	}
	return result, err
}
