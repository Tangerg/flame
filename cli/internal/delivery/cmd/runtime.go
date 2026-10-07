package cmd

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd/render"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/spf13/cobra"
)

// RuntimeProfile is the immutable negotiated view used for command admission
// and runtime-info presentation. Runtime binding owns its construction.
type RuntimeProfile interface {
	mutation.ReplayProfile
	Discovery() protocol.DiscoverResponse
	ClientCapabilities() *protocol.ClientCapabilities
	Supports(string) bool
}

// Runtime is the command delivery surface consumed across the Cobra tree.
// Individual command implementations still accept narrower local interfaces
// when they need only one operation.
type Runtime interface {
	PrepareInput(context.Context, prompt.Message) ([]protocol.ContentBlock, error)
	ListSessions(context.Context, conversation.SessionQuery) (conversation.SessionPage, error)
	GetSession(context.Context, string) (conversation.SessionSnapshot, error)
	CreateSession(context.Context, conversation.CreateSession) (conversation.Session, error)
	UpdateSession(context.Context, conversation.UpdateSession) (conversation.Session, error)
	ForkSession(context.Context, conversation.ForkSession) (conversation.Session, error)
	DeleteSession(context.Context, conversation.DeleteSession) error
	GetRun(context.Context, string) (conversation.Run, error)
	ListRuns(context.Context, conversation.RunQuery) (conversation.RunPage, error)
	StartRun(context.Context, prompt.StartRun) (conversation.SegmentStream, error)
	ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error)
	SubscribeRun(context.Context, conversation.SubscribeRun) (conversation.SegmentStream, error)
	SteerRun(context.Context, prompt.SteerRun) (protocol.SteerRunResponse, error)
	CancelRun(context.Context, conversation.CancelRun) (conversation.RunCancellation, error)
	ListApprovalRules(context.Context, string) ([]protocol.ApprovalRule, error)
	DeleteApprovalRule(context.Context, string) error
}

func newRuntimeCommand(provider runtimeProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect the connected runtime",
	}
	command.AddCommand(newRuntimeInfoCommand(provider))
	return command
}

func newRuntimeInfoCommand(provider runtimeProvider) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "info",
		Short: "Show discovery identity, capabilities, and hard limits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profile, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			if asJSON {
				return render.WriteJSONLine(cmd.OutOrStdout(), struct {
					Discovery          protocol.DiscoverResponse    `json:"discovery"`
					ClientCapabilities *protocol.ClientCapabilities `json:"clientCapabilities,omitzero"`
				}{profile.Discovery(), profile.ClientCapabilities()})
			}
			return writeRuntimeProfile(cmd.OutOrStdout(), profile)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Write the complete profile as JSON")
	return command
}

func writeRuntimeProfile(output io.Writer, profile RuntimeProfile) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	discovery := profile.Discovery()
	capabilities := discovery.Capabilities
	client := profile.ClientCapabilities()
	rows := [][2]string{
		{"runtime", discovery.ServerInfo.Name + " " + discovery.ServerInfo.Version},
		{"protocol", discovery.ProtocolVersion},
		{"default workspace", discovery.ServerInfo.DefaultWorkspace.Path},
		{"home", discovery.ServerInfo.Home},
		{"run events", joinRuntimeCatalog(capabilities.RunEvents)},
		{"runtime topics", joinRuntimeCatalog(capabilities.RuntimeTopics)},
		{"streaming methods", joinRuntimeCatalog(capabilities.StreamingMethods)},
	}
	for _, name := range slices.Sorted(maps.Keys(capabilities.Features)) {
		feature := capabilities.Features[name]
		var flags []string
		if feature.Enabled {
			flags = append(flags, "enabled")
		} else {
			flags = append(flags, "disabled")
		}
		if feature.ClientOptIn {
			if client != nil && client.Features[name].Enabled {
				flags = append(flags, "client opt-in requested")
			} else {
				flags = append(flags, "client opt-in declined")
			}
		}
		if feature.RequiredByRunProtocol {
			flags = append(flags, "run protocol")
		}
		if profile.Supports(name) {
			flags = append(flags, "available")
		}
		rows = append(rows, [2]string{"feature " + string(name), strings.Join(flags, " · ")})
	}
	limits := capabilities.Limits
	rows = append(rows,
		[2]string{"run concurrency", formatRunConcurrency(limits.MaxConcurrentRuns)},
		[2]string{"command replay retention", (time.Duration(limits.Idempotency.RetentionSeconds) * time.Second).String()},
		[2]string{"run replay", fmt.Sprintf("%d events · %d bytes · %s", limits.RunReplay.MaxEvents, limits.RunReplay.MaxBytes, limits.RunReplay.Scope)},
		[2]string{"MCP auth retention", fmt.Sprintf("%d seconds", limits.MCPAuthorizationAttempts.RetentionSeconds)},
		[2]string{"runtime subscription", fmt.Sprintf("%d topics · %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches)},
	)
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func joinRuntimeCatalog[String ~string](values []String) string {
	items := make([]string, len(values))
	for index, value := range values {
		items[index] = string(value)
	}
	return strings.Join(items, ", ")
}

func formatRunConcurrency(maximum *int) string {
	if maximum == nil {
		return "unbounded"
	}
	return fmt.Sprintf("at most %d runs", *maximum)
}
