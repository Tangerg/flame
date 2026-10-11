import { useMemo, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk/types/tool";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewReads } from "../application/pluginView";
import type { ToolResultSnapshot } from "../adapters/toolRendererReads";
import { PackageView } from "./PackageView";

const styles = stylex.create({
  page: { height: "min(50vh, 24rem)", width: "100%" },
});

export function ToolResultPackage({
  tool,
  reads: createReads,
  ...props
}: ToolPreviewProps & {
  reads: (tool: ToolPreviewProps["tool"]) => PluginViewReads<ToolResultSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
  fallback: ReactNode;
}) {
  const reads = useMemo(() => createReads(tool), [createReads, tool]);
  return (
    <div {...stylex.props(styles.page)}>
      <PackageView {...props} reads={reads} />
    </div>
  );
}
