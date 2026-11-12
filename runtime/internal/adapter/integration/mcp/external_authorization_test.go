package mcp

import (
	"math"
	"testing"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestExternalAuthorizationStartKeepsItsOriginalServerAndPendingState(t *testing.T) {
	id, err := app.ParseAuthorizationAttemptID("mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	server := testsupport.UserMCPServer("external")
	created := time.Unix(100, 0).UTC()
	attempt := wire.Attempt{ID: id.String(), Name: "external", Status: wire.Pending, StartedAt: created.Format(time.RFC3339Nano)}
	if _, err := readStartedExternalAttempt(wire.AttemptResult{Attempt: &attempt}, id, server); err != nil {
		t.Fatalf("pending original attempt: %v", err)
	}
	other := attempt
	other.Name = "other"
	if _, err := readStartedExternalAttempt(wire.AttemptResult{Attempt: &other}, id, server); err == nil {
		t.Fatal("authorization start accepted another server's attempt")
	}
	for _, status := range []wire.Outcome{wire.Succeeded, wire.Failed, wire.Canceled, wire.Unconfirmed} {
		t.Run(string(status), func(t *testing.T) {
			terminal := attempt
			terminal.Status = status
			terminal.FinishedAt = created.Add(time.Second).Format(time.RFC3339Nano)
			if _, err := readStartedExternalAttempt(wire.AttemptResult{Attempt: &terminal}, id, server); err == nil {
				t.Fatal("authorization start accepted a terminal attempt")
			}
		})
	}
}

func TestExternalAuthorizationAdmissionRequiresCoherentLifecycleTimes(t *testing.T) {
	id, err := app.ParseAuthorizationAttemptID("mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Unix(100, 0).UTC()
	finish := created.Add(time.Second)
	for _, status := range []wire.Outcome{wire.Pending, wire.Succeeded, wire.Failed, wire.Canceled, wire.Unconfirmed} {
		t.Run(string(status), func(t *testing.T) {
			attempt := wire.Attempt{ID: id.String(), Name: "external", Status: status, StartedAt: created.Format(time.RFC3339Nano)}
			if status != wire.Pending {
				attempt.FinishedAt = finish.Format(time.RFC3339Nano)
			}
			if _, err := readExternalAttempt(wire.AttemptResult{Attempt: &attempt}, id); err != nil {
				t.Fatalf("valid lifecycle was refused: %v", err)
			}
			for _, invalid := range []struct {
				name   string
				change func(*wire.Attempt)
			}{
				{"missing_creation", func(a *wire.Attempt) { a.StartedAt = time.Time{}.Format(time.RFC3339Nano) }},
				{"unrepresentable_creation", func(a *wire.Attempt) {
					a.StartedAt = time.Unix(0, math.MaxInt64).Add(time.Nanosecond).Format(time.RFC3339Nano)
				}},
				{"inconsistent_finish", func(a *wire.Attempt) {
					if status == wire.Pending {
						a.FinishedAt = finish.Format(time.RFC3339Nano)
					} else {
						a.FinishedAt = ""
					}
				}},
				{"finish_precedes_creation", func(a *wire.Attempt) { a.FinishedAt = created.Add(-time.Second).Format(time.RFC3339Nano) }},
				{"unrepresentable_finish", func(a *wire.Attempt) {
					a.FinishedAt = time.Unix(0, math.MaxInt64).Add(time.Nanosecond).Format(time.RFC3339Nano)
				}},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					value := attempt
					invalid.change(&value)
					if _, err := readExternalAttempt(wire.AttemptResult{Attempt: &value}, id); err == nil {
						t.Fatal("provider read admitted inconsistent authorization times")
					}
				})
			}
		})
	}
}
