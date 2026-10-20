import { createElement, type ReactNode } from "react";
import { type CapturedToolResult } from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { InstalledToolResultComponent } from "@/features/plugins/ui/InstalledComponent";
import { RecordedToolComponent } from "@/features/plugins/ui/RecordedToolComponent";
import { RecordedResult } from "../../../plugins/tool-results/component/component";
import declaration from "../../../plugins/tool-results/package/io.github.tangerg.flame/declaration.json";
import { type PluginClientComponent } from "@flame/runtime-client/wire";
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
