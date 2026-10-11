package runs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type AutomationOrigin struct{ InstallationID, ReleaseDigest, ServerName string }

// AutomationStart is an immutable requested Run opening. The origin owns its
// work item; Runtime alone commits Session/Run acceptance and its replay evidence.
type AutomationStart struct {
	Origin         AutomationOrigin
	SessionID      string
	RunID          string
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
}

func (a AutomationStart) Validate() error {
	if _, err := resourceid.ParseInstallation(a.Origin.InstallationID); err != nil {
		return err
	}
	if _, err := fingerprint.ParseDigest(a.Origin.ReleaseDigest); err != nil {
		return err
	}
	if _, err := mcpserver.ParseServerName(a.Origin.ServerName); err != nil {
		return err
	}
	if err := resourceid.ValidateSession(a.SessionID); err != nil {
		return err
	}
	if err := resourceid.ValidateRun(a.RunID); err != nil {
		return err
	}
	if strings.TrimSpace(a.Instructions) == "" {
		return errors.New("automation: instructions are required")
	}
	return nil
}
func (a AutomationStart) Digest() (string, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	captured := a
	captured.Origin.ReleaseDigest = ""
	captured.Origin.ServerName = ""
	encoded, err := json.Marshal(captured, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

type AutomationReceipt struct{ RequestDigest, SessionID, RunID, SegmentID string }
type AutomationStartReader interface {
	Get(context.Context, string) (AutomationReceipt, bool, error)
}

func AcceptAutomation(request AutomationStart, opening OpeningCommit) (AutomationReceipt, error) {
	digest, err := request.Digest()
	if err != nil {
		return AutomationReceipt{}, err
	}
	admission, present := opening.Admission()
	if !present {
		return AutomationReceipt{}, errors.New("automation: missing Run admission")
	}
	return AutomationReceipt{RequestDigest: digest, SessionID: admission.SessionID, RunID: admission.RunID, SegmentID: admission.SegmentID}, nil
}
func (c *Coordinator) StartAutomation(ctx context.Context, request AutomationStart) (StartResult, error) {
	digest, err := request.Digest()
	if err != nil {
		return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
	}
	release, err := c.automationAdmissions.Acquire(ctx, request.RunID)
	if err != nil {
		return StartResult{}, err
	}
	defer release()
	receipt, found, err := c.automationStarts.Get(ctx, request.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if found {
		if receipt.RequestDigest != digest {
			return StartResult{}, errors.New("automation: Run identity already binds another request")
		}
		return StartResult{SessionID: receipt.SessionID, RunID: receipt.RunID, SegmentID: receipt.SegmentID}, nil
	}
	return c.startRoot(ctx, rootStartCommand{
		StartCommand: StartCommand{ModelSelection: request.ModelSelection, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: request.Instructions}}},
		automation:   &request, workspacePath: request.CWD,
	})
}

func (r AutomationReceipt) Validate() error {
	if _, err := fingerprint.ParseDigest(r.RequestDigest); err != nil {
		return err
	}
	if err := resourceid.ValidateSession(r.SessionID); err != nil {
		return err
	}
	if err := resourceid.ValidateRun(r.RunID); err != nil {
		return err
	}
	return resourceid.ValidateSegment(r.SegmentID)
}
