package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestOpeningReportsPromptHookRefusal(t *testing.T) {
	const refusal = "choose the workspace policy before continuing"
	const postRefusalEffect = "prompt-hook-after-refusal"
	for _, tc := range []struct {
		name    string
		event   hook.Event
		verdict hook.CommandVerdict
	}{
		{"startup denial", hook.SessionStart, hook.CommandDeny},
		{"prompt denial", hook.UserPromptSubmit, hook.CommandDeny},
		{"prompt approval request", hook.UserPromptSubmit, hook.CommandAsk},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type authoredRule struct {
				Event   hook.Event `json:"event"`
				Command string     `json:"command"`
			}
			rules := []authoredRule{{
				Event:   tc.event,
				Command: fmt.Sprintf(`printf '%%s' '{"decision":%q,"reason":%q}'`, tc.verdict, refusal),
			}}
			if tc.event == hook.SessionStart {
				rules = append(rules, authoredRule{
					Event:   hook.UserPromptSubmit,
					Command: fmt.Sprintf(`printf x > %s; printf '%%s' '{"decision":"allow"}'`, postRefusalEffect),
				})
			}
			document, err := json.Marshal(struct {
				Hooks []authoredRule `json:"hooks"`
			}{Hooks: rules})
			if err != nil {
				t.Fatal(err)
			}
			cfg := poMCPConfig(t)
			writeHookSource(t, cfg.userHome, string(document))
			runtime := poMCPOpen(t, cfg)
			activateHookProvider(t, runtime.api)
			session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
				Title: "Opening policy",
			}, "create-prompt-policy-session").(*protocol.Session)
			result := runtime.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Proceed with the task"}},
			}, protocol.CallMetadata{IdempotencyKey: "opening-policy", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
			if result.Failure == nil {
				t.Error("Run opening ignored the lifecycle policy")
				for _, err := range result.Events {
					if err != nil {
						t.Error(err)
					}
				}
			} else {
				problem := result.Failure.Problem()
				recovery, _ := protocol.RecoveryFor(problem.Type)
				if problem.Type != protocol.ErrPromptRejected.Error() || !strings.Contains(problem.Detail, refusal) || recovery != protocol.RecoveryStop {
					t.Errorf("opening failure = %+v, recovery = %q, want the original prompt refusal with stop recovery", problem, recovery)
				}
			}
			runs := runtime.must(delivery.RunsList, protocol.ListRunsRequest{SessionID: session.ID}, "").(*protocol.Page[protocol.RunRef])
			if len(runs.Data) != 0 {
				t.Errorf("refused prompt opened a Run: %+v", runs.Data)
			}
			if tc.event == hook.SessionStart {
				if _, err := os.Stat(filepath.Join(cfg.defaultWorkspacePath, postRefusalEffect)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("UserPromptSubmit executed after SessionStart rejected the opening: %v", err)
				}
			}
		})
	}
}
