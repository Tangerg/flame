import { useMemo, type ReactNode } from "react";
import { useT } from "@/lib/i18n";
import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk/types/tool";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewReads } from "../application/pluginView";
import type { ToolResultSnapshot } from "../adapters/rendererReads";
import { PackageView } from "./PackageView";

const styles = stylex.create({
  page: { height: "min(50vh, 24rem)", width: "100%" },
});

export function ToolResultPackage({
  tool,
  reads: createReads,
  fallback,
  ...props
}: ToolPreviewProps & {
  reads: (tool: ToolPreviewProps["tool"]) => PluginViewReads<ToolResultSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
  fallback: ReactNode;
}) {
  const t = useT();
  const reads = useMemo(() => createReads(tool), [createReads, tool]);
  return (
    <div {...stylex.props(styles.page)}>
      <PackageView
        {...props}
        reads={reads}
        fallback={{ label: t("toolInspector.recordedResult"), content: fallback }}
      />
    </div>
  );
}
