package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd/render"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const maximumPromptBytes = 4 << 20

func newRunCommand(provider runtimeProvider, v *viper.Viper) *cobra.Command {
	flags := new(runFlags)
	cmd := &cobra.Command{
		Use:   "run [prompt...]",
		Short: "Run one prompt to completion and exit",
		Long: "run drives a single prompt without an interactive surface: it starts a run,\n" +
			"follows it to a terminal state, writes the selected output format, and exits.\n\n" +
			"Anything piped in is appended to the prompt. --file attaches a local file as\n" +
			"typed context and can be repeated; attachment-only turns are valid.\n\n" +
			"An unattended run denies approval requests by default and continues with that\n" +
			"answer. --approve-all answers yes instead. Questions leave the run parked and\n" +
			"name the session to continue interactively.",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return flags.execute(cmd, args, provider, v)
		},
	}
	flags.register(cmd)
	_ = cmd.RegisterFlagCompletionFunc("file", completeRunFile(provider))
	_ = cmd.RegisterFlagCompletionFunc("output-format", completeOutputFormat)
	return cmd
}

type runFlags struct {
	output     string
	asJSON     bool
	approveAll bool
	sessionID  string
	files      []string
}

func (r *runFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&r.output, "output-format", "text", "Output format: text, json, or streaming-json")
	cmd.Flags().BoolVar(&r.asJSON, "json", false, "Shorthand for --output-format json")
	cmd.Flags().BoolVar(&r.approveAll, "approve-all", false, "Approve every request the run makes")
	cmd.Flags().StringVarP(&r.sessionID, "session", "s", "", "Run inside an existing session instead of a new one")
	cmd.Flags().StringArrayVarP(&r.files, "file", "f", nil, "Attach a local file (repeatable)")
}

func (r *runFlags) execute(cmd *cobra.Command, args []string, provider runtimeProvider, v *viper.Viper) error {
	format, err := r.selectedOutputFormat(cmd)
	if err != nil {
		return err
	}
	config, err := readSettings(v)
	if err != nil {
		return err
	}
	runOptions, err := config.RunOptions()
	if err != nil {
		return err
	}
	messageText, err := r.readMessageText(cmd, args)
	if err != nil {
		return err
	}

	var (
		runtime Runtime
		profile *runtimebinding.Profile
		opened  agent.SessionSnapshot
		message agent.Message
	)
	if r.sessionID == "" {
		workspacePath, workspaceErr := resolveWorkspace(cmd)
		if workspaceErr != nil {
			return workspaceErr
		}
		message, err = r.buildMessage(cmd.Context(), messageText, workspacePath)
		if err != nil {
			return err
		}
		runtime, profile, err = provider.Open(cmd)
		if err != nil {
			return err
		}
		opened, err = session.Open(cmd.Context(), runtime, "", workspacePath)
	} else {
		runtime, profile, err = provider.Open(cmd)
		if err != nil {
			return err
		}
		opened, err = session.Open(cmd.Context(), runtime, r.sessionID, "")
		if err == nil {
			message, err = r.buildMessage(cmd.Context(), messageText, opened.Session.Workspace.Path)
		}
	}
	if err != nil {
		return err
	}
	replayPolicy, err := runtimebinding.CommandReplayPolicy(profile)
	if err != nil {
		return fmt.Errorf("runtime command replay policy: %w", err)
	}
	return runworkflow.Execute(cmd.Context(), runworkflow.Invocation{
		Runtime:  runtime,
		Renderer: newRunRenderer(cmd, format),
		Start: agent.StartRun{
			SessionID: opened.Session.ID,
			Message:   message,
			Options:   runOptions,
		},
		ApproveAll:        r.approveAll,
		ReconnectAttempts: config.UI.ReconnectAttempts,
		ReplayPolicy:      replayPolicy,
	})
}

type outputFormat string

const (
	outputText          outputFormat = "text"
	outputJSON          outputFormat = "json"
	outputStreamingJSON outputFormat = "streaming-json"
)

func (r *runFlags) selectedOutputFormat(cmd *cobra.Command) (outputFormat, error) {
	selected := outputFormat(r.output)
	if r.asJSON {
		if cmd.Flags().Changed("output-format") && selected != outputJSON {
			return "", errors.New("--json conflicts with a non-JSON --output-format")
		}
		selected = outputJSON
	}
	switch selected {
	case outputText, outputJSON, outputStreamingJSON:
		return selected, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (want text, json, or streaming-json)", selected)
	}
}

func completeOutputFormat(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{
		string(outputText) + "\tstream plain text for people",
		string(outputJSON) + "\twrite one final result object",
		string(outputStreamingJSON) + "\tstream one event object per line",
	}
	matched := candidates[:0]
	for _, candidate := range candidates {
		formatName, _, _ := strings.Cut(candidate, "\t")
		if strings.HasPrefix(formatName, toComplete) {
			matched = append(matched, candidate)
		}
	}
	return matched, cobra.ShellCompDirectiveNoFileComp
}

func (r *runFlags) readMessageText(cmd *cobra.Command, args []string) (string, error) {
	text, textErr := readPrompt(cmd, args)
	if textErr != nil && (!errors.Is(textErr, errNoPrompt) || len(r.files) == 0) {
		return "", textErr
	}
	return text, nil
}

func (r *runFlags) buildMessage(ctx context.Context, text, workspace string) (agent.Message, error) {
	attached, err := resolveAttachments(ctx, workspace, r.files)
	if err != nil {
		return agent.Message{}, err
	}
	return agent.Message{Text: text, Attachments: attached}, nil
}

func newRunRenderer(cmd *cobra.Command, format outputFormat) runworkflow.Renderer {
	switch format {
	case outputJSON:
		return render.NewResultJSON(cmd.OutOrStdout())
	case outputStreamingJSON:
		return render.NewNDJSON(cmd.OutOrStdout())
	default:
		return render.NewText(cmd.OutOrStdout())
	}
}

func completeRunFile(provider runtimeProvider) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		workspacePath, err := runFileCompletionWorkspace(cmd, provider)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		resolver, err := attachment.New(workspacePath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		matches, err := resolver.Complete(cmd.Context(), toComplete)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		out := make([]string, 0, len(matches))
		for _, match := range matches {
			out = append(out, match.Path+"\t"+match.Detail)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func runFileCompletionWorkspace(cmd *cobra.Command, provider runtimeProvider) (string, error) {
	sessionID, err := cmd.Flags().GetString("session")
	if err != nil {
		return "", err
	}
	if sessionID == "" {
		return resolveWorkspace(cmd)
	}
	runtime, err := provider.Runtime(cmd)
	if err != nil {
		return "", err
	}
	snapshot, err := session.Open(cmd.Context(), runtime, sessionID, "")
	if err != nil {
		return "", err
	}
	return snapshot.Session.Workspace.Path, nil
}

func resolveAttachments(ctx context.Context, workspace string, paths []string) ([]agent.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	resolver, err := attachment.New(workspace)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]agent.Attachment, 0, len(paths))
	for _, path := range paths {
		item, err := resolver.Resolve(ctx, path)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[item.Path]; duplicate {
			continue
		}
		if len(out) >= agent.MaxMessageAttachments {
			return nil, fmt.Errorf("at most %d unique attachments are allowed", agent.MaxMessageAttachments)
		}
		seen[item.Path] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

// readPrompt assembles the prompt from arguments and anything piped in.
func readPrompt(cmd *cobra.Command, args []string) (string, error) {
	parts := make([]string, 0, 2)
	if given := strings.TrimSpace(strings.Join(args, " ")); given != "" {
		parts = append(parts, given)
	}
	piped, err := readPipedPrompt(cmd.InOrStdin())
	if err != nil {
		return "", err
	}
	if piped != "" {
		parts = append(parts, piped)
	}
	if len(parts) == 0 {
		return "", errNoPrompt
	}
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maximumPromptBytes {
		return "", fmt.Errorf("prompt exceeds the %d-byte limit", maximumPromptBytes)
	}
	if !utf8.ValidString(prompt) {
		return "", errors.New("prompt is not valid UTF-8")
	}
	return prompt, nil
}

// readPipedPrompt reads stdin when it is not a terminal. A terminal is left alone: a
// prompt-less `flame run` should say so, not sit there looking hung.
func readPipedPrompt(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok {
		info, err := f.Stat()
		if err != nil {
			return "", fmt.Errorf("inspect stdin: %w", err)
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return "", nil
		}
	}
	b, err := io.ReadAll(io.LimitReader(in, maximumPromptBytes+1))
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if len(b) > maximumPromptBytes {
		return "", fmt.Errorf("piped prompt exceeds the %d-byte limit", maximumPromptBytes)
	}
	if !utf8.Valid(b) {
		return "", errors.New("piped prompt is not valid UTF-8")
	}
	return strings.TrimSpace(string(b)), nil
}
