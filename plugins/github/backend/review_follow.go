package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/frame/sqlite"
)

const reviewPollInterval = 2 * time.Minute

type followedReview struct {
	wire.IssueInput
	scope string
}

func (c *connector) setUpReviewFollow(ctx context.Context, setup pluginmcp.Setup) (io.Closer, error) {
	for _, contribution := range setup.Contributions() {
		if contribution.Export != "github" || contribution.Background == nil {
			continue
		}
		ref, err := pluginmcp.CallbackFromContext(ctx)
		if err != nil {
			return nil, err
		}
		client, err := runtimeclient.ConnectCallback(ctx, ref)
		if err != nil {
			return nil, err
		}
		background, err := c.reviewBackground(ctx, reviewPollInterval, func(ctx context.Context) error {
			return client.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution.ID}, protocol.CallOptions{})
		}, client.Close)
		if err != nil {
			return nil, errors.Join(err, client.Close())
		}
		return background, nil
	}
	return nil, nil
}

func (c *connector) reviewBackground(lifetime context.Context, interval time.Duration, notify func(context.Context) error, close func() error) (*pluginmcp.Background, error) {
	return pluginmcp.NewBackground(lifetime, pluginmcp.BackgroundConfig{
		Run: func(ctx context.Context) {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			announce := func() {
				if err := notify(ctx); err != nil && ctx.Err() == nil {
					slog.WarnContext(ctx, "github: notify review change", "error", err)
				}
			}
			for {
				c.pollReviews(ctx, announce)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-c.reviewWake:
				}
			}
		},
		Release: close,
	})
}

func (c *connector) followedReviews(ctx context.Context) ([]followedReview, error) {
	var targets []followedReview
	err := c.store.db.Read(ctx, func(reader sqlite.Reader) error {
		rows, err := reader.QueryContext(ctx, `SELECT scope,json_extract(snapshot,'$.follow.repository'),json_extract(snapshot,'$.follow.number')
			FROM review_work WHERE json_type(snapshot,'$.follow')='object' ORDER BY scope`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var target followedReview
			if err := rows.Scan(&target.scope, &target.Repository, &target.Number); err != nil {
				return err
			}
			if target.scope == c.reviewScope(target.Repository, target.Number) {
				targets = append(targets, target)
			}
		}
		return rows.Err()
	})
	return targets, err
}

func (c *connector) pollReviews(ctx context.Context, announce func()) {
	targets, err := c.followedReviews(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "github: load followed reviews", "error", err)
		}
		return
	}
	for _, target := range targets {
		err := c.pollReview(ctx, target, announce)
		if ctx.Err() != nil {
			return
		}
		message := ""
		if err != nil {
			message = "Could not check the followed pull request."
			if refusal, known := errors.AsType[*framecall.Refusal](err); known {
				message = refusal.Message
			}
			slog.WarnContext(ctx, "github: follow pull request", "repository", target.Repository, "pull", target.Number, "error", err)
		}
		_, changed, saveErr := c.updateFollowSnapshot(ctx, target.scope, func(snapshot *reviewSnapshot) (bool, error) {
			if snapshot.Follow == nil || !snapshot.Follow.Enabled || snapshot.Follow.Error == message {
				return false, nil
			}
			snapshot.Follow.Error = message
			return true, nil
		})
		if saveErr != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "github: save follow state", "error", saveErr)
		}
		if changed && saveErr == nil {
			announce()
		}
	}
}

func (c *connector) pollReview(ctx context.Context, target followedReview, announce func()) error {
	snapshot, err := c.store.reviewSnapshot(ctx, target.scope)
	if err != nil || snapshot.Follow == nil {
		return err
	}
	pending := pendingReviewRecheck(snapshot)
	if pending < 0 && !snapshot.Follow.Enabled {
		return nil
	}
	pull, err := c.getPull(ctx, target.IssueInput)
	if err != nil {
		return err
	}
	if pending < 0 {
		if snapshot.Latest == pull.Base.SHA+":"+pull.Head.SHA {
			return nil
		}
		input := wire.ReviewWorkInput{IssueInput: target.IssueInput}
		observed, err := c.inspectReview(ctx, input, pull)
		if err != nil {
			return err
		}
		version := snapshot.Version
		updated, changed, err := c.updateFollowSnapshot(ctx, target.scope, func(current *reviewSnapshot) (bool, error) {
			if current.Version != version || current.Follow == nil || !current.Follow.Enabled {
				return false, nil
			}
			_, err := current.observe(input, observed)
			return err == nil, err
		})
		if err != nil || !changed {
			return err
		}
		snapshot = updated
		pending = pendingReviewRecheck(snapshot)
		announce()
	}
	if pending < 0 || ctx.Err() != nil {
		return ctx.Err()
	}
	revision := snapshot.Revisions[pending].ReviewVersion
	var previous reviewRevision
	if index := snapshot.revision(revision.Recheck.PreviousRevision); index >= 0 {
		previous = snapshot.Revisions[index]
	}
	repository, err := newRepository(target.Repository)
	if err != nil {
		return err
	}
	key := c.credential.cacheKey("review worktree\n" + target.scope + "\n" + revision.ID)
	result := c.reviewGit.recheck(ctx, key, c.credential, repository, revision, previous)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, changed, err := c.updateFollowSnapshot(ctx, target.scope, func(current *reviewSnapshot) (bool, error) {
		index := current.revision(revision.ID)
		if index < 0 || current.Revisions[index].Recheck == nil || current.Revisions[index].Recheck.Status != wire.ReviewRecheckPending {
			return false, nil
		}
		current.Revisions[index].Recheck = &result
		return true, nil
	})
	if changed && err == nil {
		announce()
	}
	return err
}

func pendingReviewRecheck(snapshot reviewSnapshot) int {
	return slices.IndexFunc(snapshot.Revisions, func(revision reviewRevision) bool {
		return revision.Recheck != nil && revision.Recheck.Status == wire.ReviewRecheckPending
	})
}

func (c *connector) updateFollowSnapshot(ctx context.Context, scope string, change func(*reviewSnapshot) (bool, error)) (snapshot reviewSnapshot, changed bool, err error) {
	err = c.store.db.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		snapshot, err = c.store.reviewSnapshot(ctx, scope)
		if err != nil {
			return err
		}
		changed, err = change(&snapshot)
		if err != nil || !changed {
			return err
		}
		snapshot.Version++
		encoded, err := encodeReviewSnapshot(snapshot)
		if err != nil {
			return err
		}
		return c.store.saveReviewSnapshot(ctx, scope, encoded)
	})
	return snapshot, changed, err
}
