import { AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { useScheme, type Scheme } from "@/lib/appearance";
import { asyncDisposeSymbol } from "dougong";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { useEffect, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { TrajectoryViewReads, TrajectoryViewStatus } from "../application/trajectoryView";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { mountTrajectoryFrame } from "../adapters/trajectoryFrame";
import { space, color, type as typeStep } from "@/styles/tokens.stylex";

const styles = stylex.create({
  root: { width: "100%", height: "100%" },
  identity: {
    paddingInline: space.s4,
    paddingBlock: space.s2,
    color: color.fgMuted,
    flexShrink: 0,
  },
  carrier: { flex: 1, minHeight: 0, width: "100%" },
  status: { padding: space.s4, color: color.fgMuted },
});
export function PackageView(props: {
  reads: (sessionId: string, includeDescendants: boolean) => TrajectoryViewReads;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const sessionId = useActiveSessionId();
  const includeDescendants = useRuntimeCapability("subagents");
  const scheme = useScheme();
  if (!sessionId)
    return (
      <p role="status" {...stylex.props(styles.status)}>
        {t("packages.view.selectSession")}
      </p>
    );
  return (
    <SessionView
      key={`${sessionId}:${includeDescendants}:${scheme}`}
      {...props}
      sessionId={sessionId}
      includeDescendants={includeDescendants}
      scheme={scheme}
    />
  );
}
function SessionView({
  reads,
  lifetime,
  sessionId,
  title,
  carrier,
  includeDescendants,
  scheme,
}: {
  reads: (sessionId: string, includeDescendants: boolean) => TrajectoryViewReads;
  lifetime: ContributionLifetime;
  sessionId: string;
  includeDescendants: boolean;
  scheme: Scheme;
  title: string;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const container = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<TrajectoryViewStatus>({ type: "loading" });
  useEffect(() => {
    const instance = lifetime.lifetime(`session:${sessionId}`);
    const signal = instance.signal;
    if (container.current)
      instance.cleanup(
        mountTrajectoryFrame({
          container: container.current,
          reads: reads(sessionId, includeDescendants),
          carrier,
          signal,
          scheme,
          status: (value) => {
            if (!signal.aborted) setStatus(value);
          },
        }),
      );
    return () => {
      void instance[asyncDisposeSymbol]().catch(console.error);
    };
  }, [reads, sessionId, includeDescendants, scheme, carrier, lifetime]);
  return (
    <AgentWorkspaceView ariaLabel={title} className={stylex.props(styles.root).className}>
      <div {...stylex.props(styles.identity, typeStep.uiSm)}>
        {t("packages.view.identity", { title })}
      </div>
      {status.type === "loading" && (
        <p role="status" {...stylex.props(styles.status)}>
          {t("packages.view.loading")}
        </p>
      )}
      {status.type === "failure" && (
        <p role="alert" {...stylex.props(styles.status)}>
          {status.reason}
        </p>
      )}
      <div ref={container} {...stylex.props(styles.carrier)} />
    </AgentWorkspaceView>
  );
}
