import { useMemo } from "react";
import { useT } from "@/lib/i18n";
import type { TrajectoryEntry } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import type { PluginViewReads } from "../application/pluginView";
import { PackageView } from "./PackageView";
export function TrajectoryPackageView(props: {
  reads: (sessionId: string, includeDescendants: boolean) => PluginViewReads<TrajectoryEntry>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const sessionId = useActiveSessionId();
  const includeDescendants = useRuntimeCapability("subagents");

  if (!sessionId) return <p role="status">{t("packages.view.selectSession")}</p>;
  return (
    <BoundTrajectoryView
      key={`${sessionId}:${includeDescendants}`}
      {...props}
      sessionId={sessionId}
      includeDescendants={includeDescendants}
    />
  );
}
function BoundTrajectoryView(props: {
  reads: (sessionId: string, includeDescendants: boolean) => PluginViewReads<TrajectoryEntry>;
  sessionId: string;
  includeDescendants: boolean;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const { reads: createReads, sessionId, includeDescendants } = props;
  const reads = useMemo(
    () => createReads(sessionId, includeDescendants),
    [createReads, sessionId, includeDescendants],
  );
  return <PackageView {...props} reads={reads} />;
}
