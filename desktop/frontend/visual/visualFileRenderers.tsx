import { createElement, type ReactNode } from "react";
import type { CapturedFile } from "@flame/plugin-sdk";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginClientComponent } from "@flame/runtime-contract/wire";
import { definePlugin, WORKSPACE_FILE_RENDERER } from "@/plugins/sdk";
import { InstalledFileComponent } from "@/plugins/builtin/settings/plugins-pane/ui/InstalledComponent";
import { RecordedFileComponent } from "@/plugins/builtin/settings/plugins-pane/ui/RecordedFileComponent";
import { Markdown } from "../../../plugins/file-renderers/src/component";
import declaration from "../../../plugins/file-renderers/io.github.tangerg.flame/declaration.json";
import { visualComponentProps } from "./installedComponentProps";

export function visualFileRenderers(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.visual.file-renderers",
    setup(ctx) {
      for (const entry of declaration.components as PluginClientComponent[]) {
        if (entry.placement !== "fileRenderer" || entry.selector?.type !== "fileExtension")
          continue;
        const lifetime = ctx.lifetime(entry.id);
        const sources = visualComponentProps(runtimeClient(), lifetime, entry.id);
        const File = ({ file, fallback }: { file: CapturedFile; fallback: ReactNode }) =>
          createElement(InstalledFileComponent, { ...sources, file, fallback, entry: Markdown });
        lifetime.contribute(
          WORKSPACE_FILE_RENDERER,
          {
            id: `visual:${entry.id}`,
            title: entry.title,
            extension: entry.selector.name,
            component: (props) => createElement(RecordedFileComponent, { ...props, entry: File }),
          },
          { key: entry.id },
        );
      }
    },
  });
}
