package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
)

func (a *app) ShowPlugins() {
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	statuses := a.pluginHost.Statuses()
	lines := make([]string, 0, len(statuses)+len(a.pluginIssues))
	for _, status := range statuses {
		lines = append(lines, formatPluginStatus(status))
	}
	for _, issue := range a.pluginIssues {
		lines = append(lines, fmt.Sprintf("failed   source:%s · %v", issue.Source, issue.Err))
	}
	a.transcript.Append(&kit.Entry{Theme: a.transcript.theme, Label: "plugins", Body: strings.Join(lines, "\n")})
}

// reportUnloadedPlugins says at startup that something did not load. A plugin
// that fails discovery, resolution or setup contributes nothing, so without
// this notice its absence is indistinguishable from a plugin never installed.
func (a *app) reportUnloadedPlugins(results []extensions.LifecycleResult, issues []extensions.SourceIssue) {
	unloaded := len(issues)
	for _, result := range results {
		if result.Phase != extensions.PluginLoaded {
			unloaded++
		}
	}
	switch unloaded {
	case 0:
	case 1:
		a.message("1 plugin did not load · /plugins for details")
	default:
		a.message(fmt.Sprintf("%d plugins did not load · /plugins for details", unloaded))
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
	case status.Trusted && status.Capabilities == nil:
		return "unrestricted"
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

func (a *app) ReloadPlugin(id string) {
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	affected, err := a.pluginHost.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	results, err := a.pluginHost.Reload(id)
	a.registerCommands()
	if err != nil {
		a.message(err.Error())
		return
	}
	var failed []extensions.LifecycleResult
	for _, result := range results {
		if result.Err != nil {
			failed = append(failed, result)
		}
	}
	// The status row holds one message, so several failures point at /plugins
	// rather than letting the last one overwrite the rest.
	switch len(failed) {
	case 0:
		a.message("reloaded plugin " + id)
	case 1:
		a.message(fmt.Sprintf("plugin %s · %s · %v", failed[0].PluginID, failed[0].Phase, failed[0].Err))
	default:
		a.message(fmt.Sprintf("%d plugins did not reload · /plugins for details", len(failed)))
	}
}

func (a *app) UnloadPlugin(id string) {
	if a.pluginHost == nil {
		a.message("plugin host is unavailable")
		return
	}
	id = strings.TrimSpace(id)
	for _, status := range a.pluginHost.Statuses() {
		if status.ID == id && status.Trusted {
			a.message("built-in plugin " + id + " can be reloaded but not unloaded")
			return
		}
	}
	affected, err := a.pluginHost.Affected(id)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.cancelPluginCommands(affected...)
	err = a.pluginHost.Unload(id)
	a.registerCommands()
	if err != nil {
		a.message(err.Error())
		return
	}
	a.message("unloaded plugin " + id)
}
