import * as stylex from "@stylexjs/stylex";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import {
  TOOL_RESULT_RENDERER,
  type ToolPreviewProps,
  type ToolResultRendererSpec,
} from "@/features/chat/public/tools/types";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { ToolInspector, ToolResultLayout } from "@/features/chat/tools/ui/ToolInspector";
import { useCallback, type ReactNode } from "react";
import { useContentVisible } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { ToolText } from "@/ui/agent/index";
import { type ToolCall } from "@/features/agent/public/sessionView";
import {
  useRendererChoice,
  useRendererResolver,
  useRendererTitle,
} from "@/features/plugins/public/renderers";
import {
  toolRendererCandidates,
  toolRendererTarget,
} from "@/features/chat/tools/application/toolResultRenderer";

const styles = stylex.create({
  controls: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: space.s2,
  },
});

export interface StandaloneToolResult {
  renderer?: ToolResultRendererSpec;
}

export type StandaloneToolResultFor = (tool: ToolCall) => StandaloneToolResult | undefined;

export function useStandaloneToolResult(): StandaloneToolResultFor {
  const renderers = useExtensionPoint(TOOL_RESULT_RENDERER);
  const resolve = useRendererResolver("tool");
  const hasInline = renderers.some((renderer) => renderer.inlineResult);
  return useCallback(
    (tool) => {
      if (!hasInline || tool.status !== "ok") return undefined;
      const candidates = toolRendererCandidates(renderers, tool);
      if (!candidates.some((candidate) => candidate.inlineResult)) return undefined;
      // Keep the disclosure and its picker in place when the renderer preference changes.
      const renderer = resolve(toolRendererTarget(tool), candidates);
      return { renderer: renderer?.inlineResult ? renderer : undefined };
    },
    [hasInline, renderers, resolve],
  );
}

function RenderToolResult({
  tool,
  renderer,
  fallback,
}: ToolPreviewProps & { renderer: ToolResultRendererSpec; fallback: ReactNode }) {
  const titleFor = useRendererTitle();
  const Result = renderer.component;
  return (
    <PluginBoundary
      key={renderer.id}
      plugin={renderer.id}
      label={titleFor(renderer)}
      fallback={fallback}
    >
      <Result tool={tool} fallback={fallback} />
    </PluginBoundary>
  );
}

export function InlineToolResult({
  tool,
  renderer,
}: ToolPreviewProps & { renderer: ToolResultRendererSpec }) {
  const titleFor = useRendererTitle();
  const visible = useContentVisible();
  return (
    <ToolResultLayout tool={tool} label={titleFor(renderer)} visible={visible}>
      <RenderToolResult tool={tool} renderer={renderer} fallback={null} />
    </ToolResultLayout>
  );
}

export function ToolPreview({ tool, raw }: ToolPreviewProps & { raw?: boolean }) {
  const renderers = useExtensionPoint(TOOL_RESULT_RENDERER);
  const { renderer, control } = useRendererChoice(
    "tool",
    toolRendererTarget(tool),
    toolRendererCandidates(renderers, tool),
  );
  return (
    <>
      {(raw || control) && (
        <div {...stylex.props(styles.controls)}>
          {raw && <ToolText value={{ kind: "machine", value: tool.name }} wrap />}
          {control}
        </div>
      )}
      <ToolInspector tool={tool} collapseResult={raw}>
        {renderer && !raw
          ? (fallback) => <RenderToolResult tool={tool} renderer={renderer} fallback={fallback} />
          : undefined}
      </ToolInspector>
    </>
  );
}
