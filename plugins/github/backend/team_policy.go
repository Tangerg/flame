package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	teamSecretEnv   = "FLAME_GITHUB_TEAM_SECRET"
	teamOriginEnv   = "FLAME_GITHUB_TEAM_ORIGIN"
	intakeEnv       = "FLAME_GITHUB_WORK_INTAKE"
	linearSecretEnv = "FLAME_LINEAR_SIGNING_SECRET"
	slackSecretEnv  = "FLAME_SLACK_SIGNING_SECRET"
)

type teamPolicy struct {
	origin string
	secret []byte
}

type intakePolicy struct {
	ProjectID          string `json:"projectId"`
	LinearOrganization string `json:"linearOrganization,omitempty"`
	LinearTeam         string `json:"linearTeam,omitempty"`
	SlackTeam          string `json:"slackTeam,omitempty"`
	SlackChannel       string `json:"slackChannel,omitempty"`
	SlackOrigin        string `json:"slackOrigin,omitempty"`
	linearSecret       []byte
	slackSecret        []byte
}

func (c *connector) configureTeam() error {
	origin, secret := os.Getenv(teamOriginEnv), os.Getenv(teamSecretEnv)
	if origin != "" || secret != "" {
		base, err := bareWebOrigin(origin, true)
		if err != nil || len(secret) < 32 || len(secret) > maxTokenBytes || c.issuePolicy == nil {
			return errors.New("github: team sharing requires an HTTPS origin, a signing secret and an Issue delivery project")
		}
		c.teamPolicy = &teamPolicy{origin: base, secret: []byte(secret)}
	}
	raw := os.Getenv(intakeEnv)
	linear, slack := os.Getenv(linearSecretEnv), os.Getenv(slackSecretEnv)
	if raw == "" && linear == "" && slack == "" {
		return nil
	}
	var intake intakePolicy
	if err := json.Unmarshal([]byte(raw), &intake, json.RejectUnknownMembers(true)); err != nil {
		return errors.New("github: invalid work intake policy")
	}
	if _, err := identity.ParseProject(intake.ProjectID); err != nil {
		return errors.New("github: work intake requires a project ID")
	}
	if intake.LinearOrganization != "" || intake.LinearTeam != "" || linear != "" {
		if !intakeIdentity(intake.LinearOrganization) || !intakeIdentity(intake.LinearTeam) || !validSigningSecret(linear) {
			return errors.New("github: Linear intake requires one organization, one team and a signing secret")
		}
		intake.linearSecret = []byte(linear)
	}
	if intake.SlackTeam != "" || intake.SlackChannel != "" || intake.SlackOrigin != "" || slack != "" {
		origin, err := bareWebOrigin(intake.SlackOrigin, false)
		if err != nil || !intakeIdentity(intake.SlackTeam) || !intakeIdentity(intake.SlackChannel) || !validSigningSecret(slack) {
			return errors.New("github: Slack intake requires one workspace, one channel, an HTTPS permalink origin and a signing secret")
		}
		u, _ := url.Parse(origin)
		if !strings.HasSuffix(u.Hostname(), ".slack.com") || u.Port() != "" {
			return errors.New("github: Slack permalink origin must be a slack.com workspace")
		}
		intake.SlackOrigin, intake.slackSecret = origin, []byte(slack)
	}
	if len(intake.linearSecret) == 0 && len(intake.slackSecret) == 0 {
		return errors.New("github: choose a configured source for work intake")
	}
	c.intakePolicy = &intake
	return nil
}

func validSigningSecret(secret string) bool {
	return len(secret) >= 16 && len(secret) <= maxTokenBytes
}

func bareWebOrigin(raw string, loopback bool) (string, error) {
	endpoint, err := httporigin.ParseEndpoint(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u := endpoint.URL()
	if (u.Path != "" && u.Path != "/") || endpoint.HasQuery() || endpoint.HasFragment() ||
		(u.Scheme != "https" && !(loopback && endpoint.Loopback())) {
		return "", errors.New("an HTTPS origin is required")
	}
	return u.Scheme + "://" + u.Host, nil
}

func intakeIdentity(value string) bool {
	return len(value) > 0 && len(value) <= 128 && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) == -1
}

func signText(secret []byte, pieces ...[]byte) []byte {
	mac := hmac.New(sha256.New, secret)
	for _, piece := range pieces {
		_, _ = mac.Write(piece)
	}
	return mac.Sum(nil)
}

func validSignature(secret []byte, signed string, pieces ...[]byte) bool {
	if len(secret) == 0 || len(signed) != sha256.Size*2 {
		return false
	}
	digest, err := hex.DecodeString(signed)
	return err == nil && hmac.Equal(digest, signText(secret, pieces...))
}
