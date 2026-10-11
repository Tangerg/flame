import { useMemo } from "react";
import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import type { WorkspaceFileRendererProps } from "@/plugins/sdk/types/workspace";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewReads } from "../application/pluginView";
import type { FileSnapshot } from "../adapters/rendererReads";
import { PackageView } from "./PackageView";

const styles = stylex.create({ page: { height: "min(65vh, 36rem)", width: "100%" } });

export function FilePackageRenderer({
  cwd,
  path,
  content,
  startLine,
  totalLines,
  truncated,
  fallback,
  reads: createReads,
  ...props
}: WorkspaceFileRendererProps & {
  reads: (file: FileSnapshot["file"]) => PluginViewReads<FileSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const reads = useMemo(
    () => createReads({ cwd, path, content, startLine, totalLines, truncated }),
    [createReads, cwd, path, content, startLine, totalLines, truncated],
  );
  return (
    <div {...stylex.props(styles.page)}>
      <PackageView
        {...props}
        reads={reads}
        fallback={{ label: t("file.mode.source"), content: fallback }}
      />
    </div>
  );
}
