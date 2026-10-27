package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

type commentBody struct {
	Body string `json:"body"`
}

func (c *connector) createComment(ctx context.Context, input rpc.CreateCommentInput) (rpc.Comment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.Comment{}, err
	}
	if !textPresent(input.Body) {
		return rpc.Comment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", repository.apiPath(), input.Number), commentBody{input.Body}, http.StatusCreated, commentWire.admit)
}

func commentPath(input rpc.CommentInput, kind string) (string, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return "", err
	}
	if input.CommentID <= 0 {
		return "", refused("invalid_input", "a positive comment ID is required")
	}
	return fmt.Sprintf("%s/%s/comments/%d", repository.apiPath(), kind, input.CommentID), nil
}

func (c *connector) updateComment(ctx context.Context, input rpc.UpdateCommentInput) (rpc.Comment, error) {
	path, err := commentPath(input.CommentInput, "issues")
	if err != nil {
		return rpc.Comment{}, err
	}
	if !textPresent(input.Body) {
		return rpc.Comment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPatch, path, commentBody{input.Body}, http.StatusOK, func(wire commentWire) (rpc.Comment, error) {
		if wire.ID != input.CommentID {
			return rpc.Comment{}, errors.New("github: updated comment ID differs from original target")
		}
		return wire.admit()
	})
}

func (c *connector) deleteComment(ctx context.Context, input rpc.CommentInput) (rpc.DeletedComment, error) {
	return c.deleteCommentKind(ctx, input, "issues")
}

func (c *connector) deleteReviewComment(ctx context.Context, input rpc.CommentInput) (rpc.DeletedComment, error) {
	return c.deleteCommentKind(ctx, input, "pulls")
}

func (c *connector) deleteCommentKind(ctx context.Context, input rpc.CommentInput, kind string) (rpc.DeletedComment, error) {
	path, err := commentPath(input, kind)
	if err != nil {
		return rpc.DeletedComment{}, err
	}
	if _, err := c.sendWrite(ctx, http.MethodDelete, path, nil, http.StatusNoContent); err != nil {
		return rpc.DeletedComment{}, err
	}
	return rpc.DeletedComment{ID: input.CommentID, Deleted: true}, nil
}

func inlineComment(input rpc.InlineComment) (inlineCommentWire, error) {
	if _, err := newFilePath(input.Path); err != nil {
		return inlineCommentWire{}, err
	}
	if !textPresent(input.Body) || input.Line <= 0 || input.Side != rpc.Left && input.Side != rpc.Right {
		return inlineCommentWire{}, refused("invalid_input", "inline comment requires a nonblank body and positive line with LEFT or RIGHT side")
	}
	if (input.StartLine == nil) != (input.StartSide == nil) {
		return inlineCommentWire{}, refused("invalid_input", "a comment range requires both startLine and startSide")
	}
	if input.StartLine != nil && (*input.StartLine <= 0 || *input.StartSide != rpc.Left && *input.StartSide != rpc.Right || *input.StartSide == input.Side && *input.StartLine >= input.Line) {
		return inlineCommentWire{}, refused("invalid_input", "inline comment range is invalid")
	}
	return inlineCommentWire{Path: input.Path, Body: input.Body, Line: input.Line, Side: input.Side, StartLine: input.StartLine, StartSide: input.StartSide}, nil
}

type inlineCommentWire struct {
	Path      string        `json:"path"`
	Body      string        `json:"body"`
	Line      int64         `json:"line"`
	Side      rpc.DiffSide  `json:"side"`
	StartLine *int64        `json:"start_line,omitzero"`
	StartSide *rpc.DiffSide `json:"start_side,omitzero"`
}

func (c *connector) createReviewComment(ctx context.Context, input rpc.CreateReviewCommentInput) (rpc.ReviewComment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.ReviewComment{}, err
	}
	if !commitSHA.MatchString(input.CommitID) {
		return rpc.ReviewComment{}, refused("invalid_input", "an exact inspected commit SHA is required")
	}
	location, err := inlineComment(input.InlineComment)
	if err != nil {
		return rpc.ReviewComment{}, err
	}
	payload := struct {
		CommitID string `json:"commit_id"`
		inlineCommentWire
	}{strings.ToLower(input.CommitID), location}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/comments", repository.apiPath(), input.Number), payload, http.StatusCreated, func(wire reviewCommentWire) (rpc.ReviewComment, error) {
		if !strings.EqualFold(wire.CommitID, input.CommitID) || wire.Path != input.Path {
			return rpc.ReviewComment{}, errors.New("github: created review comment differs from inspected commit or path")
		}
		return wire.admit()
	})
}

func (c *connector) replyReviewComment(ctx context.Context, input rpc.ReplyReviewCommentInput) (rpc.ReviewComment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return rpc.ReviewComment{}, err
	}
	if input.CommentID <= 0 || !textPresent(input.Body) {
		return rpc.ReviewComment{}, refused("invalid_input", "a positive parent comment ID and nonblank reply body are required")
	}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/comments/%d/replies", repository.apiPath(), input.Number, input.CommentID), commentBody{input.Body}, http.StatusCreated, func(wire reviewCommentWire) (rpc.ReviewComment, error) {
		if wire.InReplyTo == nil || *wire.InReplyTo != input.CommentID {
			return rpc.ReviewComment{}, errors.New("github: reply belongs to a different comment thread")
		}
		return wire.admit()
	})
}

func (c *connector) updateReviewComment(ctx context.Context, input rpc.UpdateCommentInput) (rpc.ReviewComment, error) {
	path, err := commentPath(input.CommentInput, "pulls")
	if err != nil {
		return rpc.ReviewComment{}, err
	}
	if !textPresent(input.Body) {
		return rpc.ReviewComment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPatch, path, commentBody{input.Body}, http.StatusOK, func(wire reviewCommentWire) (rpc.ReviewComment, error) {
		if wire.ID != input.CommentID {
			return rpc.ReviewComment{}, errors.New("github: updated comment ID differs from original target")
		}
		return wire.admit()
	})
}
