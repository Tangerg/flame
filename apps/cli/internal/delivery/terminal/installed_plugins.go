package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (a *app) ShowPlugins() {
	a.executeRuntimeReaderQuery(a.pluginsReaderQuery())
}

func (a *app) pluginsReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading installed plugins", mode: runtimeReaderPlugins,
		read: func(ctx context.Context) (readerDocument, error) {
			installations, err := a.plugins.ListPlugins(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return installedPluginsDocument(installations), nil
		},
	}
}

func installedPluginsDocument(installations []protocol.PluginInstallation) readerDocument {
	if len(installations) == 0 {
		return paragraphDocument("Installed plugins", "none installed", []string{"No Runtime plugins installed."})
	}
	sections := make([]toolSection, 0, len(installations))
	for _, installation := range installations {
		selected := installation.Selected
		title := selected.Name
		if selected.Version != "" {
			title += "@" + selected.Version
		}
		lines := []string{
			"installation  " + installation.ID,
			"selected      " + selected.Digest,
			fmt.Sprintf("enabled       %t", installation.Enabled),
			"presentation  " + string(installation.Presentation),
			"realization   " + string(installation.Realization.Type),
			"source        " + installation.Source,
		}
		if installation.Staged != nil {
			lines = append(lines, "staged        "+installation.Staged.Digest)
		}
		for _, backend := range installation.Realization.UnavailableBackends {
			lines = append(lines, "unavailable   "+backend)
		}
		for _, diagnostic := range selected.Diagnostics {
			component := string(diagnostic.Component.Type)
			if diagnostic.Component.Name != nil {
				component += "/" + *diagnostic.Component.Name
			}
			lines = append(lines, fmt.Sprintf("diagnostic    %s · %s", component, diagnostic.Code))
		}
		sections = append(sections, toolSection{Title: title, Style: toolSectionCode, Text: strings.Join(lines, "\n")})
	}
	return readerDocument{Title: "Installed plugins", Detail: fmt.Sprintf("%d installed", len(installations)), Sections: sections}
}
