package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd/render"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func newRunsCommand(provider runtimeProvider) *cobra.Command {
	command := &cobra.Command{
		Use:     "runs",
		Short:   "Inspect and control durable runs",
		Aliases: []string{"run-history"},
		Args:    cobra.NoArgs,
	}
	command.AddCommand(
		newRunsListCommand(provider),
		newRunsShowCommand(provider),
		newRunsCancelCommand(provider),
	)
	return command
}

func newRunsListCommand(provider runtimeProvider) *cobra.Command {
	flags := new(runsListFlags)
	command := &cobra.Command{
		Use:          "ls",
		Short:        "List runs, newest first",
		Aliases:      []string{"list"},
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return flags.execute(cmd, provider)
		},
	}
	flags.register(command)
	_ = command.RegisterFlagCompletionFunc("session", completeSessionIDs(provider))
	_ = command.RegisterFlagCompletionFunc("status", completeRunStatus)
	return command
}

type runsListFlags struct {
	sessionID          string
	statusNames        []string
	includeDescendants bool
	cursor             string
	limit              int
	asJSON             bool
}

func (r *runsListFlags) register(command *cobra.Command) {
	command.Flags().StringVarP(&r.sessionID, "session", "s", "", "Only runs from this session")
	command.Flags().StringSliceVar(&r.statusNames, "status", nil, "Only these statuses: running, waiting, finished (repeatable)")
	command.Flags().BoolVar(&r.includeDescendants, "include-descendants", false, "Include subagent child runs")
	command.Flags().StringVar(&r.cursor, "cursor", "", "Opaque cursor returned by the previous page")
	command.Flags().IntVarP(
		&r.limit,
		"limit",
		"n",
		agent.DefaultPageRows,
		fmt.Sprintf("Maximum runs to return (up to %d)", agent.MaximumPageRows),
	)
	command.Flags().BoolVar(&r.asJSON, "json", false, "Write the page as JSON")
}

func (r *runsListFlags) execute(cmd *cobra.Command, provider runtimeProvider) error {
	statuses, err := parseRunStatuses(r.statusNames)
	if err != nil {
		return err
	}
	pageSize, err := pageSizeFromFlag(cmd, "limit", r.limit)
	if err != nil {
		return err
	}
	query := agent.RunQuery{
		SessionID: r.sessionID, Statuses: statuses, IncludeDescendants: r.includeDescendants,
		Cursor: r.cursor, PageSize: pageSize,
	}
	if err := query.Validate(); err != nil {
		return err
	}
	runtime, profile, err := provider.Open(cmd)
	if err != nil {
		return err
	}
	if r.includeDescendants && profile != nil &&
		!profile.Supports(protocol.FeatureSubagents) {
		return fmt.Errorf("runtime capability %q was not negotiated", protocol.FeatureSubagents)
	}
	page, err := runtime.ListRuns(cmd.Context(), query)
	if err != nil {
		return err
	}
	if err := page.Validate(); err != nil {
		return fmt.Errorf("list runs: %w", err)
	}
	if r.asJSON {
		return render.WriteRunPageJSON(cmd.OutOrStdout(), page)
	}
	if err := writeRunList(cmd, page); err != nil {
		return err
	}
	if page.NextCursor != "" {
		_, err = fmt.Fprintf(cmd.ErrOrStderr(), "more runs: --cursor %s\n", page.NextCursor)
	}
	return err
}

func writeRunList(cmd *cobra.Command, page agent.RunPage) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, run := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			run.ID, runScope(run), run.Status, runModel(run), run.SessionID,
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func newRunsShowCommand(provider runtimeProvider) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:          "show <run-id>",
		Short:        "Show one authoritative run projection",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := provider.Runtime(cmd)
			if err != nil {
				return err
			}
			run, err := runtime.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := run.Validate(); err != nil {
				return fmt.Errorf("show run: %w", err)
			}
			if asJSON {
				return render.WriteRunJSON(cmd.OutOrStdout(), run)
			}
			return writeRunDetails(cmd, run)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Write the run as JSON")
	command.ValidArgsFunction = completeFirstRunArgument(provider)
	return command
}

func writeRunDetails(cmd *cobra.Command, run agent.Run) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	rows := [][2]string{
		{"id", run.ID},
		{"session", run.SessionID},
		{"scope", runScope(run)},
		{"status", string(run.Status)},
		{"model", runModel(run)},
	}
	if run.ActiveSegmentID != "" {
		rows = append(rows, [2]string{"segment", run.ActiveSegmentID})
	}
	if run.Status == protocol.RunStatusFinished {
		outcome := string(run.Outcome.Status)
		if detail := run.Outcome.Description(); detail != "" {
			outcome += " · " + detail
		}
		rows = append(rows, [2]string{"outcome", outcome})
	}
	rows = append(rows, [2]string{"tokens", fmt.Sprintf("%d in · %d out · %d cached", run.Usage.InputTokens, run.Usage.OutputTokens, run.Usage.CacheReadTokens)})
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func newRunsCancelCommand(provider runtimeProvider) *cobra.Command {
	var (
		reason string
		yes    bool
		asJSON bool
	)
	command := &cobra.Command{
		Use:          "cancel <run-id>",
		Short:        "Cancel a root run or one child subtree",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to cancel a run without --yes")
			}
			runtime, profile, err := provider.Open(cmd)
			if err != nil {
				return err
			}
			commandID, err := agent.NewCommandID()
			if err != nil {
				return fmt.Errorf("prepare run cancellation: %w", err)
			}
			request := agent.CancelRun{CommandID: commandID, RunID: args[0], Reason: reason}
			replayPolicy, err := runtimebinding.CommandReplayPolicy(profile)
			if err != nil {
				return fmt.Errorf("runtime command replay policy: %w", err)
			}
			replay, err := replayPolicy.NewGuard()
			if err != nil {
				return fmt.Errorf("prepare run cancellation replay guard: %w", err)
			}
			result, err := mutation.ConfirmAdmitted(cmd.Context(), mutation.AcknowledgementBackoff(), mutation.FreshReplayAdmission(replayPolicy, replay), func(ctx context.Context) (agent.RunCancellation, error) {
				return runtime.CancelRun(ctx, request)
			})
			if err != nil {
				return err
			}
			if validateTargetErr := result.ValidateTarget(args[0]); validateTargetErr != nil {
				return fmt.Errorf("cancel run: %w", validateTargetErr)
			}
			if asJSON {
				return render.WriteRunCancellationJSON(cmd.OutOrStdout(), result)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "canceled\t%s\t%s\nroot\t%s\t%s\n",
				result.Canceled.ID, result.Canceled.Outcome.Status, result.Root.ID, result.Root.Status,
			)
			return err
		},
	}
	command.Flags().StringVar(&reason, "reason", "canceled from the CLI", "Record why the run was canceled")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm cancellation")
	command.Flags().BoolVar(&asJSON, "json", false, "Write the cancellation result as JSON")
	command.ValidArgsFunction = completeFirstRunArgument(provider)
	return command
}

func parseRunStatuses(names []string) ([]protocol.RunStatus, error) {
	if len(names) == 0 {
		return nil, nil
	}
	statuses := make([]protocol.RunStatus, 0, len(names))
	for _, name := range names {
		status := protocol.RunStatus(strings.TrimSpace(name))
		if status == "" {
			return nil, errors.New("run status is empty")
		}
		statuses = append(statuses, status)
	}
	query := agent.RunQuery{Statuses: statuses, PageSize: agent.DefaultPageSize()}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return statuses, nil
}

func completeRunStatus(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	items := []string{
		string(protocol.RunStatusRunning) + "\texecuting a segment",
		string(protocol.RunStatusWaiting) + "\twaiting for human input",
		string(protocol.RunStatusFinished) + "\tterminal run",
	}
	return filterCompletionPrefix(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeFirstRunArgument(provider runtimeProvider) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		runtime, profile, err := provider.Open(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		includeDescendants := profile == nil ||
			profile.Supports(protocol.FeatureSubagents)
		page, err := runtime.ListRuns(cmd.Context(), agent.RunQuery{
			IncludeDescendants: includeDescendants, PageSize: agent.MaximumPageSize(),
		})
		if err != nil || page.Validate() != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		items := make([]string, 0, len(page.Items))
		for _, run := range page.Items {
			if toComplete != "" && !strings.HasPrefix(run.ID, toComplete) {
				continue
			}
			items = append(items, fmt.Sprintf("%s\t%s · %s · %s", run.ID, runScope(run), run.Status, runModel(run)))
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}
}

func filterCompletionPrefix(items []string, prefix string) []string {
	filtered := items[:0]
	for _, item := range items {
		value, _, _ := strings.Cut(item, "\t")
		if strings.HasPrefix(value, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func runScope(run agent.Run) string {
	if run.Lineage.IsRoot() {
		return "root"
	}
	return "child of " + run.Lineage.ParentRunID()
}

func runModel(run agent.Run) string {
	if run.Provider == "" {
		return "-"
	}
	return run.Provider + "/" + run.Model
}
