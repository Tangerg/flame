package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/spf13/cobra"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// Runtime is the command delivery surface consumed across the Cobra tree.
// Individual command implementations still accept narrower local interfaces
// when they need only one operation.
type Runtime interface {
	ListSessions(context.Context, agent.SessionQuery) (agent.SessionPage, error)
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
	CreateSession(context.Context, agent.CreateSession) (agent.Session, error)
	UpdateSession(context.Context, agent.UpdateSession) (agent.Session, error)
	ForkSession(context.Context, agent.ForkSession) (agent.Session, error)
	RollbackSession(context.Context, agent.RollbackSession) (agent.RollbackResult, error)
	DeleteSession(context.Context, agent.DeleteSession) error
	GetRun(context.Context, string) (agent.Run, error)
	ListRuns(context.Context, agent.RunQuery) (agent.RunPage, error)
	StartRun(context.Context, agent.StartRun) (agent.SegmentStream, error)
	ResumeRun(context.Context, agent.ResumeRun) (agent.SegmentStream, error)
	SubscribeRun(context.Context, agent.SubscribeRun) (agent.SegmentStream, error)
	SteerRun(context.Context, agent.SteerRun) error
	CancelRun(context.Context, agent.CancelRun) (agent.RunCancellation, error)
	ListModels(context.Context) ([]protocol.Model, error)
	GetApprovalMode(context.Context) (protocol.ApprovalMode, error)
	SetApprovalMode(context.Context, protocol.ApprovalMode) (protocol.ApprovalMode, error)
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
			if profile == nil {
				return errors.New("runtime discovery profile is unavailable")
			}
			projected := profile.Clone()
			if err := projected.Validate(); err != nil {
				return err
			}
			if asJSON {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(projected)
			}
			return writeRuntimeProfile(cmd.OutOrStdout(), projected)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Write the complete profile as JSON")
	return command
}

func writeRuntimeProfile(output io.Writer, profile runtimebinding.Profile) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	rows := [][2]string{
		{"runtime", profile.Server.Name + " " + profile.Server.Version},
		{"protocol", profile.Protocol.Version},
		{"default workspace", profile.Server.DefaultWorkspace},
		{"home", profile.Server.Home},
		{"run events", strings.Join(profile.RunEvents, ", ")},
		{"runtime topics", strings.Join(profile.RuntimeTopics, ", ")},
		{"streaming methods", strings.Join(profile.StreamingMethods, ", ")},
	}
	for _, name := range slices.Sorted(maps.Keys(profile.Features)) {
		feature := profile.Features[name]
		var flags []string
		if feature.Enabled {
			flags = append(flags, "enabled")
		} else {
			flags = append(flags, "disabled")
		}
		if feature.ClientOptIn {
			if feature.ClientRequested {
				flags = append(flags, "client opt-in requested")
			} else {
				flags = append(flags, "client opt-in declined")
			}
		}
		if feature.RequiredByRunProtocol {
			flags = append(flags, "run protocol")
		}
		if feature.Available() {
			flags = append(flags, "available")
		}
		rows = append(rows, [2]string{"feature " + string(name), strings.Join(flags, " · ")})
	}
	limits := profile.Limits
	rows = append(rows,
		[2]string{"run concurrency", formatRunConcurrency(limits.RunConcurrency)},
		[2]string{"command replay retention", limits.CommandReplay.Retention().String()},
		[2]string{"run replay", fmt.Sprintf("%d events · %d bytes · %s", limits.RunReplay.MaxEvents, limits.RunReplay.MaxBytes, limits.RunReplay.Scope)},
		[2]string{"MCP auth retention", fmt.Sprintf("%d seconds", limits.MCPAuthorizationRetentionSeconds)},
		[2]string{"runtime subscription", fmt.Sprintf("%d topics · %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches)},
	)
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func formatRunConcurrency(limit runtimebinding.RunConcurrencyLimit) string {
	maximum, bounded := limit.Maximum()
	if !bounded {
		return "unbounded"
	}
	return fmt.Sprintf("at most %d runs", maximum)
}
