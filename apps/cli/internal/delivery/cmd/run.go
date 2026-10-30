package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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
	completeFlag(cmd, "file", completeRunFile(provider))
	completeFlag(cmd, "output-format", completeOutputFormat)
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
	cmd.Flags().String("workspace", "", "Workspace path on the selected Runtime (default: local directory when embedded, Runtime default when remote)")
	cmd.MarkFlagsMutuallyExclusive("session", "workspace")
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
		profile RuntimeProfile
		opened  conversation.SessionSnapshot
		message authoring.Message
	)
	if r.sessionID == "" {
		workspacePath, workspaceErr := resolveWorkspace(cmd, config)
		if workspaceErr != nil {
			return workspaceErr
		}
		attachmentRoot := workspacePath
		if config.Runtime.Mode == settings.RuntimeRemote {
			attachmentRoot, err = resolveLocalDirectory(cmd)
			if err != nil {
				return err
			}
		}
		message, err = r.buildMessage(cmd.Context(), messageText, attachmentRoot)
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
			attachmentRoot := opened.Session.Workspace.Path
			if config.Runtime.Mode == settings.RuntimeRemote {
				attachmentRoot, err = resolveLocalDirectory(cmd)
				if err != nil {
					return err
				}
			}
			message, err = r.buildMessage(cmd.Context(), messageText, attachmentRoot)
		}
	}
	if err != nil {
		return err
	}
	replayPolicy, err := mutation.PolicyFromProfile(profile, time.Now)
	if err != nil {
		return fmt.Errorf("runtime command replay policy: %w", err)
	}
	return runworkflow.Execute(cmd.Context(), runworkflow.Invocation{
		Runtime:  runtime,
		Renderer: newRunRenderer(cmd, format),
		Start: authoring.StartRun{
			SessionID: opened.Session.ID,
			Message:   message,
			Options:   runOptions,
		},
		ApproveAll:   r.approveAll,
		ReplayPolicy: replayPolicy,
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
	return filterCompletionPrefix(candidates, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func (r *runFlags) readMessageText(cmd *cobra.Command, args []string) (string, error) {
	text, textErr := readPrompt(cmd, args)
	if textErr != nil && (!errors.Is(textErr, errNoPrompt) || len(r.files) == 0) {
		return "", textErr
	}
	return text, nil
}

func (r *runFlags) buildMessage(ctx context.Context, text, workspace string) (authoring.Message, error) {
	attached, err := resolveAttachments(ctx, workspace, r.files)
	if err != nil {
		return authoring.Message{}, err
	}
	return authoring.Message{Text: text, Attachments: attached}, nil
}

func newRunRenderer(cmd *cobra.Command, format outputFormat) runworkflow.Renderer {
	switch format {
	case outputJSON:
		return newResultJSONRenderer(cmd.OutOrStdout())
	case outputStreamingJSON:
		return newNDJSONRenderer(cmd.OutOrStdout())
	default:
		return newTextRenderer(cmd.OutOrStdout())
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
	if err := provider.prepare(cmd); err != nil {
		return "", err
	}
	config, err := readSettings(provider.configuration)
	if err != nil {
		return "", err
	}
	if config.Runtime.Mode == settings.RuntimeRemote {
		return resolveLocalDirectory(cmd)
	}
	sessionID, err := cmd.Flags().GetString("session")
	if err != nil {
		return "", err
	}
	if sessionID == "" {
		return resolveWorkspace(cmd, config)
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

func resolveAttachments(ctx context.Context, workspace string, paths []string) ([]authoring.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	resolver, err := attachment.New(workspace)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]authoring.Attachment, 0, len(paths))
	for _, path := range paths {
		item, err := resolver.Resolve(ctx, path)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[item.Path]; duplicate {
			continue
		}
		if len(out) >= authoring.MaxMessageAttachments {
			return nil, fmt.Errorf("at most %d unique attachments are allowed", authoring.MaxMessageAttachments)
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
	piped, err := readPipedPrompt(cmd.Context(), cmd.InOrStdin())
	if err != nil {
		return "", err
	}
	if piped != "" {
		parts = append(parts, piped)
	}
	if len(parts) == 0 {
		return "", errNoPrompt
	}
	authoredPrompt := strings.Join(parts, "\n\n")
	if len(authoredPrompt) > authoring.MaxMessageTextBytes {
		return "", fmt.Errorf("prompt exceeds the %d-byte limit", authoring.MaxMessageTextBytes)
	}
	if !utf8.ValidString(authoredPrompt) {
		return "", errors.New("prompt is not valid UTF-8")
	}
	return authoredPrompt, nil
}

// readPipedPrompt reads stdin when it is not a terminal. A terminal is left alone: a
// prompt-less `flame run` should say so, not sit there looking hung.
func readPipedPrompt(ctx context.Context, in io.Reader) (_ string, err error) {
	reader, closeReader, err := promptReader(ctx, in)
	if err != nil || reader == nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, closeReader())
		if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
			err = errors.Join(cause, err)
		}
	}()
	b, err := io.ReadAll(io.LimitReader(reader, authoring.MaxMessageTextBytes+1))
	if cause := context.Cause(ctx); cause != nil {
		return "", errors.Join(cause, err)
	}
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if len(b) > authoring.MaxMessageTextBytes {
		return "", fmt.Errorf("piped prompt exceeds the %d-byte limit", authoring.MaxMessageTextBytes)
	}
	if !utf8.Valid(b) {
		return "", errors.New("piped prompt is not valid UTF-8")
	}
	return strings.TrimSpace(string(b)), nil
}
