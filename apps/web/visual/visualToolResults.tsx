import { createElement, type ReactNode } from "react";
import { type CapturedToolResult, type PluginComponent } from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { InstalledToolResultComponent } from "@/features/plugins/ui/InstalledComponent";
import { RecordedToolComponent } from "@/features/plugins/ui/RecordedToolComponent";
import { RecordedResult } from "../../../plugins/tool-results/component/component";
import declaration from "../../../plugins/tool-results/package/io.github.tangerg.flame/declaration.json";
import genuiDeclaration from "../../../plugins/genui/package/io.github.tangerg.flame/declaration.json";
import { type PluginClientComponent } from "@flame/runtime-client/wire";
import { visualComponentProps } from "./installedComponentProps";
const releasedGenUI = import.meta.glob<Record<string, PluginComponent>>(
  "../../../plugins/genui/package/**/components/component.js",
);

export async function visualToolResults(runtimeClient: () => FlameClient, genui = false) {
  const renderers = (declaration.components as PluginClientComponent[]).map((entry) => ({
    key: entry.id,
    declaration: entry,
    component: RecordedResult as PluginComponent,
  }));
  if (genui) {
    for (const entry of genuiDeclaration.components as PluginClientComponent[]) {
      const load = releasedGenUI[`../../../plugins/genui/package/${entry.entry}`];
      if (!load) throw new Error("genui fixture requires its built release component");
      const component = (await load())[entry.export];
      if (!component) throw new Error("genui release component export is missing");
      renderers.push({ key: `genui:${entry.id}`, declaration: entry, component });
    }
  }
  return definePlugin({
    name: "flame.visual.tool-results",
    setup(ctx) {
      for (const renderer of renderers) {
        const entry = renderer.declaration;
        if (!entry.selector || entry.placement !== "toolResultRenderer") continue;
        const lifetime = ctx.lifetime(renderer.key);
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
            entry: renderer.component,
          });
        lifetime.contribute(
          TOOL_RESULT_RENDERER,
          {
            id: `visual:${renderer.key}`,
            title: entry.title,
            selector: entry.selector,
            component: (props) =>
              createElement(RecordedToolComponent, {
                ...props,
                entry: Result,
              }),
          },
          { key: renderer.key },
        );
      }
    },
  });
}
