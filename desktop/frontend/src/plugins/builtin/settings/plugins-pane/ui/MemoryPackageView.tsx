import { useMemo, useState } from "react";
import type { AgentMemoryItem } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useT } from "@/lib/i18n";
import { PillButton, SystemMessage, vocab } from "@/ui";
import * as stylex from "@stylexjs/stylex";
import { space } from "@/styles/tokens.stylex";
import type { PluginViewReads, MemoryViewTarget } from "../application/pluginView";
import { PackageView } from "./PackageView";
import { MemoryActions } from "./MemoryActions";

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  controls: { display: "flex", gap: space.s2, padding: space.s3, flexShrink: 0 },
  page: { flex: 1, minHeight: 0 },
});
type Props = {
  reads: (target: MemoryViewTarget) => PluginViewReads<AgentMemoryItem>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
};
export function MemoryPackageView(props: Props) {
  const t = useT();
  const [scope, setScope] = useState<MemoryViewTarget["scope"]>("project");
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const projectPath = scope === "project" ? cwd : undefined;
  const target = useMemo<MemoryViewTarget | undefined>(
    () =>
      scope === "user"
        ? { scope }
        : projectPath
          ? { scope, workspace: { path: projectPath } }
          : undefined,
    [scope, projectPath],
  );
  const available = useRuntimeCapability("agentMemory");
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <div {...stylex.props(styles.controls)}>
        {(["project", "user"] as const).map((value) => (
          <PillButton
            key={value}
            size="sm"
            variant={scope === value ? "solid" : "outlined"}
            onClick={() => setScope(value)}
          >
            {t(value === "project" ? "agentMemory.scope.project" : "agentMemory.scope.user")}
          </PillButton>
        ))}
      </div>
      {!available ? (
        <SystemMessage>{t("agentMemory.unavailable.title")}</SystemMessage>
      ) : !target ? (
        <SystemMessage>
          {t(
            workspace.status === "resolving"
              ? "packages.view.loading"
              : "agentMemory.noProject.sub",
          )}
        </SystemMessage>
      ) : (
        <BoundMemoryView
          key={`${target.scope}:${target.workspace?.path ?? ""}`}
          {...props}
          target={target}
        />
      )}
    </div>
  );
}
function BoundMemoryView({ target, ...props }: Props & { target: MemoryViewTarget }) {
  const [refreshVersion, setRefreshVersion] = useState(0);
  const { reads: createReads } = props;
  const reads = useMemo(() => createReads(target), [createReads, target]);
  return (
    <>
      <MemoryActions
        scope={target.scope}
        cwd={target.workspace?.path}
        signal={props.lifetime.signal}
        onSaved={() => setRefreshVersion((value) => value + 1)}
      />
      <div {...stylex.props(styles.page)}>
        <PackageView key={refreshVersion} {...props} reads={reads} />
      </div>
    </>
  );
}
