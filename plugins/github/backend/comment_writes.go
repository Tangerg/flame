package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
)

type commentBody struct {
	Body string `json:"body"`
}

func (c *connector) createComment(ctx context.Context, input contractwire.CreateCommentInput) (contractwire.Comment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.Comment{}, err
	}
	if !textPresent(input.Body) {
		return contractwire.Comment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", repository.apiPath(), input.Number), commentBody{input.Body}, http.StatusCreated, commentWire.admit)
}

func commentPath(input contractwire.CommentInput, kind string) (string, error) {
	repository, err := newRepository(input.Repository)
	if err != nil {
		return "", err
	}
	if input.CommentID <= 0 {
		return "", refused("invalid_input", "a positive comment ID is required")
	}
	return fmt.Sprintf("%s/%s/comments/%d", repository.apiPath(), kind, input.CommentID), nil
}

func (c *connector) updateComment(ctx context.Context, input contractwire.UpdateCommentInput) (contractwire.Comment, error) {
	path, err := commentPath(input.CommentInput, "issues")
	if err != nil {
		return contractwire.Comment{}, err
	}
	if !textPresent(input.Body) {
		return contractwire.Comment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPatch, path, commentBody{input.Body}, http.StatusOK, func(wire commentWire) (contractwire.Comment, error) {
		if wire.ID != input.CommentID {
			return contractwire.Comment{}, errors.New("github: updated comment ID differs from original target")
		}
		return wire.admit()
	})
}

func (c *connector) deleteComment(ctx context.Context, input contractwire.CommentInput) (contractwire.DeletedComment, error) {
	return c.deleteCommentKind(ctx, input, "issues")
}

func (c *connector) deleteReviewComment(ctx context.Context, input contractwire.CommentInput) (contractwire.DeletedComment, error) {
	return c.deleteCommentKind(ctx, input, "pulls")
}

func (c *connector) deleteCommentKind(ctx context.Context, input contractwire.CommentInput, kind string) (contractwire.DeletedComment, error) {
	path, err := commentPath(input, kind)
	if err != nil {
		return contractwire.DeletedComment{}, err
	}
	if _, err := c.sendWrite(ctx, http.MethodDelete, path, nil, http.StatusNoContent); err != nil {
		return contractwire.DeletedComment{}, err
	}
	return contractwire.DeletedComment{ID: input.CommentID, Deleted: true}, nil
}

func inlineComment(input contractwire.InlineComment) (inlineCommentWire, error) {
	if _, err := newFilePath(input.Path); err != nil {
		return inlineCommentWire{}, err
	}
	if !textPresent(input.Body) || input.Line <= 0 || input.Side != contractwire.Left && input.Side != contractwire.Right {
		return inlineCommentWire{}, refused("invalid_input", "inline comment requires a nonblank body and positive line with LEFT or RIGHT side")
	}
	if (input.StartLine == nil) != (input.StartSide == nil) {
		return inlineCommentWire{}, refused("invalid_input", "a comment range requires both startLine and startSide")
	}
	if input.StartLine != nil && (*input.StartLine <= 0 || *input.StartSide != contractwire.Left && *input.StartSide != contractwire.Right || *input.StartSide == input.Side && *input.StartLine >= input.Line) {
		return inlineCommentWire{}, refused("invalid_input", "inline comment range is invalid")
	}
	return inlineCommentWire{Path: input.Path, Body: input.Body, Line: input.Line, Side: input.Side, StartLine: input.StartLine, StartSide: input.StartSide}, nil
}

type inlineCommentWire struct {
	Path      string                 `json:"path"`
	Body      string                 `json:"body"`
	Line      int64                  `json:"line"`
	Side      contractwire.DiffSide  `json:"side"`
	StartLine *int64                 `json:"start_line,omitzero"`
	StartSide *contractwire.DiffSide `json:"start_side,omitzero"`
}

func (c *connector) createReviewComment(ctx context.Context, input contractwire.CreateReviewCommentInput) (contractwire.ReviewComment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.ReviewComment{}, err
	}
	if !commitSHA.MatchString(input.CommitID) {
		return contractwire.ReviewComment{}, refused("invalid_input", "an exact inspected commit SHA is required")
	}
	location, err := inlineComment(input.InlineComment)
	if err != nil {
		return contractwire.ReviewComment{}, err
	}
	payload := struct {
		CommitID string `json:"commit_id"`
		inlineCommentWire
	}{strings.ToLower(input.CommitID), location}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/comments", repository.apiPath(), input.Number), payload, http.StatusCreated, func(wire reviewCommentWire) (contractwire.ReviewComment, error) {
		if !strings.EqualFold(wire.CommitID, input.CommitID) || wire.Path != input.Path {
			return contractwire.ReviewComment{}, errors.New("github: created review comment differs from inspected commit or path")
		}
		return wire.admit()
	})
}

func (c *connector) replyReviewComment(ctx context.Context, input contractwire.ReplyReviewCommentInput) (contractwire.ReviewComment, error) {
	repository, err := targetRepository(input.IssueInput)
	if err != nil {
		return contractwire.ReviewComment{}, err
	}
	if input.CommentID <= 0 || !textPresent(input.Body) {
		return contractwire.ReviewComment{}, refused("invalid_input", "a positive parent comment ID and nonblank reply body are required")
	}
	return writeValue(ctx, c, http.MethodPost, fmt.Sprintf("%s/pulls/%d/comments/%d/replies", repository.apiPath(), input.Number, input.CommentID), commentBody{input.Body}, http.StatusCreated, func(wire reviewCommentWire) (contractwire.ReviewComment, error) {
		if wire.InReplyTo == nil || *wire.InReplyTo != input.CommentID {
			return contractwire.ReviewComment{}, errors.New("github: reply belongs to a different comment thread")
		}
		return wire.admit()
	})
}

func (c *connector) updateReviewComment(ctx context.Context, input contractwire.UpdateCommentInput) (contractwire.ReviewComment, error) {
	path, err := commentPath(input.CommentInput, "pulls")
	if err != nil {
		return contractwire.ReviewComment{}, err
	}
	if !textPresent(input.Body) {
		return contractwire.ReviewComment{}, refused("invalid_input", "comment body must be nonblank")
	}
	return writeValue(ctx, c, http.MethodPatch, path, commentBody{input.Body}, http.StatusOK, func(wire reviewCommentWire) (contractwire.ReviewComment, error) {
		if wire.ID != input.CommentID {
			return contractwire.ReviewComment{}, errors.New("github: updated comment ID differs from original target")
		}
		return wire.admit()
	})
}
