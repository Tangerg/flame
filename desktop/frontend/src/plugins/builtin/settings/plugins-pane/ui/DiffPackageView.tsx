import { useMemo } from "react";
import * as stylex from "@stylexjs/stylex";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  selectWorkspaceDiffMode,
  useWorkspaceDiffMode,
  useWorkspaceFileFocus,
} from "@/plugins/builtin/workspace/public/navigation";
import { Segmented, SystemMessage, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";
import type { PluginViewReads, DiffViewTarget, DiffViewSnapshot } from "../application/pluginView";
import { PackageView } from "./PackageView";

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  controls: { padding: space.s3, flexShrink: 0 },
  page: { flex: 1, minHeight: 0 },
});

export function DiffPackageView({
  reads: createReads,
  ...props
}: {
  reads: (target: DiffViewTarget) => PluginViewReads<DiffViewSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const available = useRuntimeCapability("git");
  const mode = useWorkspaceDiffMode();
  const focus = useWorkspaceFileFocus();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const target = useMemo<DiffViewTarget | undefined>(
    () => (cwd ? { workspace: { path: cwd }, mode, focusPath: focus.path } : undefined),
    [cwd, mode, focus],
  );
  const reads = useMemo(() => (target ? createReads(target) : undefined), [createReads, target]);
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <Segmented
          value={mode}
          options={[
            { value: "worktree", label: t("diff.mode.worktree") },
            { value: "base", label: t("diff.mode.branch") },
          ]}
          onChange={(diffMode) => selectWorkspaceDiffMode(diffMode)}
          ariaLabel={t("diff.baselineAria")}
        />
      </div>
      {!available ? (
        <SystemMessage>{t("vcs.gitNotAvailable")}</SystemMessage>
      ) : !reads ? (
        <SystemMessage>
          {t(workspace.status === "resolving" ? "packages.view.loading" : "diff.noProject")}
        </SystemMessage>
      ) : (
        <div {...stylex.props(styles.page)}>
          <PackageView key={`${cwd}:${mode}:${focus.revision}`} {...props} reads={reads} />
        </div>
      )}
    </div>
  );
}
