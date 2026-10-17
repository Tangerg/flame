import { createElement, type ReactNode } from "react";
import type { CapturedToolResult } from "@flame/plugin-sdk";
import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin, TOOL_RESULT_RENDERER } from "@/plugins/sdk";
import { InstalledToolResultComponent } from "@/plugins/builtin/settings/plugins-pane/ui/InstalledComponent";
import { RecordedToolComponent } from "@/plugins/builtin/settings/plugins-pane/ui/RecordedToolComponent";
import { RecordedResult } from "../../../plugins/tool-results/src/component";
import declaration from "../../../plugins/tool-results/io.github.tangerg.flame/declaration.json";
import type { PluginClientComponent } from "@flame/runtime-contract/wire";
import { visualComponentProps } from "./installedComponentProps";

export function visualToolResults(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.visual.tool-results",
    setup(ctx) {
      for (const entry of declaration.components as PluginClientComponent[]) {
        if (!entry.selector || entry.placement !== "toolResultRenderer") continue;
        const lifetime = ctx.lifetime(entry.id);
        const sources = visualComponentProps(runtimeClient(), lifetime, entry.id);
        const Result = ({
          toolResult,
          fallback,
        }: {
          toolResult: CapturedToolResult;
          fallback: ReactNode;
        }) =>
          createElement(InstalledToolResultComponent, {
            ...sources,
            toolResult,
            fallback,
            entry: RecordedResult,
          });
        lifetime.contribute(
          TOOL_RESULT_RENDERER,
          {
            id: `visual:${entry.id}`,
            title: entry.title,
            selector: entry.selector,
            component: (props) =>
              createElement(RecordedToolComponent, {
                ...props,
                entry: Result,
              }),
          },
          { key: entry.id },
        );
      }
    },
  });
}
