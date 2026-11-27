package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

const (
	reviewWebhookPort   = "GITHUB_WEBHOOK_PORT"
	reviewWebhookSecret = "GITHUB_WEBHOOK_SECRET"
	reviewWebhookPath   = "/github/review-webhook"
	maxWebhookBytes     = 2 << 20
)

type reviewWebhook struct {
	port   int
	secret string
}

func (c *connector) configureReview() error {
	policies, err := readCheckPolicies(os.Getenv(projectChecksVariable))
	if err != nil {
		return err
	}
	c.checkPolicies = policies
	port, secret := os.Getenv(reviewWebhookPort), os.Getenv(reviewWebhookSecret)
	if port == "" && secret == "" {
		return nil
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 || strings.TrimSpace(secret) == "" || len(secret) > maxTokenBytes {
		return errors.New("github: webhook requires a loopback port and a nonempty signing secret")
	}
	c.webhook = reviewWebhook{port: value, secret: secret}
	return nil
}

func (c *connector) startReviewWebhook(ctx context.Context) (func() error, error) {
	if c.webhook.port == 0 {
		return nil, nil
	}
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.webhook.port)))
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+reviewWebhookPath, c.receiveReviewWebhook)
	server := &http.Server{
		Handler:           mux,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       8 * time.Second,
		WriteTimeout:      9 * time.Second,
		IdleTimeout:       15 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		err := server.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
		return errors.Join(err, <-done)
	}, nil
}

type reviewDelivery struct {
	Action string `json:"action"`
	Number int64  `json:"number"`
	Before string `json:"before"`
	Pull   struct {
		Number int64       `json:"number"`
		Title  string      `json:"title"`
		Base   *branchWire `json:"base"`
		Head   *branchWire `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (c *connector) receiveReviewWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		status := http.StatusBadRequest
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		w.WriteHeader(status)
		return
	}
	signature, valid := strings.CutPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if !valid || len(signature) != 2*sha256.Size {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	digest, err := hex.DecodeString(signature)
	mac := hmac.New(sha256.New, []byte(c.webhook.secret))
	_, _ = mac.Write(raw)
	if err != nil || !hmac.Equal(mac.Sum(nil), digest) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var event reviewDelivery
	if err := json.Unmarshal(raw, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if event.Action != "synchronize" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	repository, version, err := event.admit()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	changed, err := c.retainReviewDelivery(r.Context(), repository, event.Number, strings.ToLower(event.Before), version)
	if err != nil {
		slog.WarnContext(r.Context(), "github: retain review delivery", "repository", repository, "pull", event.Number, "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !changed {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	c.wakeReviews()
	w.WriteHeader(http.StatusAccepted)
}

func (d reviewDelivery) admit() (string, wire.ReviewVersion, error) {
	if d.Number <= 0 || d.Pull.Number != d.Number || d.Pull.Base == nil || d.Pull.Head == nil ||
		strings.TrimSpace(d.Pull.Title) == "" || len(d.Pull.Title) > 1024 {
		return "", wire.ReviewVersion{}, errors.New("github: malformed review delivery")
	}
	repository, err := newRepository(d.Repository.FullName)
	if err != nil {
		return "", wire.ReviewVersion{}, err
	}
	base, err := d.Pull.Base.admit()
	if err != nil || base.Repository == nil || *base.Repository != repository.String() || !commitSHA.MatchString(base.SHA) {
		return "", wire.ReviewVersion{}, errors.New("github: review delivery has no matching base commit")
	}
	head, err := d.Pull.Head.admit()
	if err != nil || !commitSHA.MatchString(head.SHA) || len(base.SHA) != len(head.SHA) ||
		d.Before != "" && (!commitSHA.MatchString(d.Before) || len(d.Before) != len(head.SHA)) {
		return "", wire.ReviewVersion{}, errors.New("github: review delivery has no matching head commit")
	}
	baseSHA, headSHA := strings.ToLower(base.SHA), strings.ToLower(head.SHA)
	version := wire.ReviewVersion{ID: baseSHA + ":" + headSHA, BaseSHA: baseSHA, HeadSHA: headSHA, Title: d.Pull.Title}
	if head.Repository != nil {
		version.HeadRepository = *head.Repository
	}
	return repository.String(), version, nil
}

func (c *connector) retainReviewDelivery(ctx context.Context, repository string, number int64, before string, version wire.ReviewVersion) (bool, error) {
	scope := c.reviewScope(repository, number)
	_, changed, err := c.updateFollowSnapshot(ctx, scope, func(s *reviewSnapshot) (bool, error) {
		if s.Follow == nil || !s.Follow.Enabled || s.Latest == "" || s.revision(version.ID) >= 0 {
			return false, nil
		}
		previous := s.Latest
		for i := len(s.Revisions) - 1; i >= 0; i-- {
			if s.Revisions[i].HeadSHA == before {
				previous = s.Revisions[i].ID
				break
			}
		}
		index, err := s.retainRevision(version)
		if err != nil {
			return false, err
		}
		revision := &s.Revisions[index]
		revision.NeedsEvidence = true
		revision.ChecksPartial = true
		revision.scheduleRecheck(previous, c.checkPolicies[repository])
		return true, nil
	})
	return changed, err
}
