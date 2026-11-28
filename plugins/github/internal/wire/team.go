package wire

import "github.com/Tangerg/flame/packages/go-sdk/instant"

type TeamProject struct {
	ProjectID string `json:"projectId"`
}

type TeamGrantInput struct {
	TeamProject
	DeliveryID    string `json:"deliveryId" jsonschema:"pattern=^[0-9a-f]{64}$"`
	Member        string `json:"member"`
	Role          string `json:"role"`
	ProjectAccess bool   `json:"projectAccess"`
	MachineAccess bool   `json:"machineAccess"`
}

type TeamGrantRef struct {
	TeamProject
	ID string `json:"id"`
}

type TeamGrant struct {
	ID string `json:"id"`
	TeamGrantInput
	ExpiresAt instant.Timestamp  `json:"expiresAt"`
	RevokedAt *instant.Timestamp `json:"revokedAt,omitzero"`
}

type TeamInvite struct {
	URL string `json:"url"`
}

type TeamAuditEvent struct {
	GrantID string            `json:"grantId"`
	Action  string            `json:"action"`
	At      instant.Timestamp `json:"at"`
}

type WorkSource struct {
	Kind       string            `json:"kind"`
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	URL        string            `json:"url"`
	ReceivedAt instant.Timestamp `json:"receivedAt"`
}

type TeamBoard struct {
	Sharing bool             `json:"sharing"`
	Grants  []TeamGrant      `json:"grants"`
	Audit   []TeamAuditEvent `json:"audit"`
	Sources []WorkSource     `json:"sources"`
}
