package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
)

// localExtensions owns the local command extension host: the commands its
// plugins contribute, the operations those commands run, and reloading or
// unloading a plugin. commandsChanged rebuilds the command catalog.
type localExtensions struct {
	host           *extensions.Host
	issues         []extensions.SourceIssue
	running        commandOperationRegistry
	tasks          taskRunner
	session        *sessionState
	localDirectory string
	status         *statusView

	notify          func(string)
	appendNote      func(label, body string)
	closed          func() bool
	commandsChanged func()
}

// registerCommands adds every contributed command through add.
func (e *localExtensions) registerCommands(add func(owner string, descriptor extensions.CommandDescriptor, run func(string, func()), available func() extensions.CommandAvailability) error) {
	for _, contributed := range e.host.Commands() {
		command := contributed.Command
		pluginID := contributed.PluginID
		var available func() extensions.CommandAvailability
		if command.Available != nil {
			available = func() extensions.CommandAvailability {
				return command.Available(extensions.CommandRequest{
					Workspace: e.session.current.Workspace.Path, SessionID: e.session.current.ID,
					LocalDirectory: authoringDirectory(e.localDirectory, e.session.current.Workspace.Path),
				})
			}
		}
		if err := add(pluginID, command.Descriptor,
			func(argument string, accept func()) { e.executeCommand(pluginID, command, argument, accept) },
			available,
		); err != nil {
			e.notify(err.Error())
		}
	}
}

func (e *localExtensions) ShowExtensions() {
	if e.host == nil {
		e.notify("plugin host is unavailable")
		return
	}
	statuses := e.host.Statuses()
	lines := make([]string, 0, len(statuses)+len(e.issues))
	for _, status := range statuses {
		lines = append(lines, formatPluginStatus(status))
	}
	for _, issue := range e.issues {
		lines = append(lines, fmt.Sprintf("failed   source:%s · %v", issue.Source, issue.Err))
	}
	if len(lines) == 0 {
		lines = append(lines, "No local command extensions installed")
	}
	e.appendNote("local command extensions", strings.Join(lines, "\n"))
}

// unloadedPluginsNotice says at startup that something did not load. A plugin
// that fails discovery, resolution or setup contributes nothing, so without
// this notice its absence is indistinguishable from a plugin never installed.
func unloadedPluginsNotice(results []extensions.LifecycleResult, issues []extensions.SourceIssue) string {
	unloaded := len(issues)
	for _, result := range results {
		if result.Phase != extensions.PluginLoaded {
			unloaded++
		}
	}
	switch unloaded {
	case 0:
		return ""
	case 1:
		return "1 plugin did not load · /extensions for details"
	default:
		return fmt.Sprintf("%d plugins did not load · /extensions for details", unloaded)
	}
}

func formatPluginStatus(status extensions.Status) string {
	line := fmt.Sprintf("%-8s %s@%s", status.Phase, status.ID, status.Version)
	line += " · capabilities " + formatCapabilities(status)
	if len(status.Requires) > 0 {
		line += " · requires " + strings.Join(status.Requires, ", ")
	}
	if status.Detail != "" {
		line += " · " + status.Detail
	}
	return line
}

func formatCapabilities(status extensions.Status) string {
	switch {
	case len(status.Capabilities) == 0:
		return "none"
	default:
		capabilities := make([]string, len(status.Capabilities))
		for i, capability := range status.Capabilities {
			capabilities[i] = string(capability)
		}
		return strings.Join(capabilities, ", ")
	}
}

func (e *localExtensions) ReloadPlugin(id string) {
	if e.host == nil {
		e.notify("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	affected, err := e.host.Affected(id)
	if err != nil {
		e.notify(err.Error())
		return
	}
	e.cancelPluginCommands(affected...)
	results, err := e.host.Reload(id)
	e.commandsChanged()
	if err != nil {
		e.notify(err.Error())
		return
	}
	var failed []extensions.LifecycleResult
	for _, result := range results {
		if result.Err != nil {
			failed = append(failed, result)
		}
	}
	// The status row holds one message, so several failures point at /extensions
	// rather than letting the last one overwrite the rest.
	switch len(failed) {
	case 0:
		e.notify("reloaded plugin " + id)
	case 1:
		e.notify(fmt.Sprintf("plugin %s · %s · %v", failed[0].PluginID, failed[0].Phase, failed[0].Err))
	default:
		e.notify(fmt.Sprintf("%d plugins did not reload · /extensions for details", len(failed)))
	}
}

func (e *localExtensions) UnloadPlugin(id string) {
	if e.host == nil {
		e.notify("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	affected, err := e.host.Affected(id)
	if err != nil {
		e.notify(err.Error())
		return
	}
	e.cancelPluginCommands(affected...)
	err = e.host.Unload(id)
	e.commandsChanged()
	if err != nil {
		e.notify(err.Error())
		return
	}
	e.notify("unloaded plugin " + id)
}
