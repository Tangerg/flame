package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

type commentWire struct {
	ID        int64     `json:"id"`
	URL       string    `json:"html_url"`
	User      *userWire `json:"user"`
	Body      *string   `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (wire commentWire) admit() (rpc.Comment, error) {
	if wire.ID <= 0 || wire.URL == "" || wire.Body == nil || wire.CreatedAt.IsZero() || wire.UpdatedAt.IsZero() {
		return rpc.Comment{}, errors.New("github: incomplete comment representation")
	}
	return rpc.Comment{ID: wire.ID, URL: wire.URL, Author: author(wire.User), Body: *wire.Body, CreatedAt: wire.CreatedAt, UpdatedAt: wire.UpdatedAt}, nil
}

func (c *connector) listComments(ctx context.Context, input rpc.ThreadInput) (rpc.CommentPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.CommentPage{}, err
	}
	path := repository.apiPath() + "/issues/" + strconv.FormatInt(input.Number, 10) + "/comments"
	page, err := readPage(ctx, c, path, pagination(input.Page, input.Limit), input.Page, input.Limit, commentWire.admit)
	return rpc.CommentPage{Comments: page.rows, NextPage: page.next}, err
}

type reviewWire struct {
	ID          int64      `json:"id"`
	URL         string     `json:"html_url"`
	User        *userWire  `json:"user"`
	Body        *string    `json:"body"`
	State       string     `json:"state"`
	CommitID    *string    `json:"commit_id"`
	SubmittedAt *time.Time `json:"submitted_at"`
}

func (wire reviewWire) admit() (rpc.Review, error) {
	if wire.ID <= 0 || wire.URL == "" || wire.Body == nil || wire.State == "" {
		return rpc.Review{}, errors.New("github: incomplete review representation")
	}
	return rpc.Review{ID: wire.ID, URL: wire.URL, Author: author(wire.User), Body: *wire.Body, State: wire.State, CommitID: wire.CommitID, SubmittedAt: wire.SubmittedAt}, nil
}

func (c *connector) listReviews(ctx context.Context, input rpc.ThreadInput) (rpc.ReviewPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.ReviewPage{}, err
	}
	path := repository.apiPath() + "/pulls/" + strconv.FormatInt(input.Number, 10) + "/reviews"
	page, err := readPage(ctx, c, path, pagination(input.Page, input.Limit), input.Page, input.Limit, reviewWire.admit)
	return rpc.ReviewPage{Reviews: page.rows, NextPage: page.next}, err
}

type reviewCommentWire struct {
	commentWire
	ReviewID         int64   `json:"pull_request_review_id"`
	Path             string  `json:"path"`
	DiffHunk         string  `json:"diff_hunk"`
	CommitID         string  `json:"commit_id"`
	OriginalCommitID string  `json:"original_commit_id"`
	Line             *int64  `json:"line"`
	OriginalLine     *int64  `json:"original_line"`
	Side             string  `json:"side"`
	StartLine        *int64  `json:"start_line"`
	StartSide        *string `json:"start_side"`
	InReplyTo        *int64  `json:"in_reply_to_id"`
}

func (wire reviewCommentWire) admit() (rpc.ReviewComment, error) {
	comment, err := wire.commentWire.admit()
	if err != nil {
		return rpc.ReviewComment{}, err
	}
	if wire.ReviewID <= 0 || wire.Path == "" || wire.CommitID == "" || wire.OriginalCommitID == "" {
		return rpc.ReviewComment{}, errors.New("github: incomplete review comment context")
	}
	return rpc.ReviewComment{Comment: comment, ReviewID: wire.ReviewID, Path: wire.Path, DiffHunk: wire.DiffHunk, CommitID: wire.CommitID, OriginalCommitID: wire.OriginalCommitID, Line: wire.Line, OriginalLine: wire.OriginalLine, Side: wire.Side, StartLine: wire.StartLine, StartSide: wire.StartSide, InReplyTo: wire.InReplyTo}, nil
}

func (c *connector) listReviewComments(ctx context.Context, input rpc.ThreadInput) (rpc.ReviewCommentPage, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return rpc.ReviewCommentPage{}, err
	}
	path := repository.apiPath() + "/pulls/" + strconv.FormatInt(input.Number, 10) + "/comments"
	parameters := pagination(input.Page, input.Limit)
	parameters.Set("sort", "created")
	parameters.Set("direction", "asc")
	page, err := readPage(ctx, c, path, parameters, input.Page, input.Limit, reviewCommentWire.admit)
	return rpc.ReviewCommentPage{Comments: page.rows, NextPage: page.next}, err
}
