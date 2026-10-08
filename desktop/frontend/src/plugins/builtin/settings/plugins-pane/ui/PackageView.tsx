import { useT } from "@/lib/i18n";
import { asyncDisposeSymbol } from "dougong";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { useEffect, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { TrajectoryViewReads } from "../application/trajectoryView";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { mountTrajectoryFrame } from "../adapters/trajectoryFrame";
import { space, color, type as typeStep } from "@/styles/tokens.stylex";

const styles = stylex.create({
  root: { width: "100%", height: "100%", minHeight: 0, display: "flex", flexDirection: "column" },
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
  reads: (sessionId: string) => TrajectoryViewReads;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const sessionId = useActiveSessionId();
  if (!sessionId)
    return (
      <p role="status" {...stylex.props(styles.status)}>
        {t("packages.view.selectSession")}
      </p>
    );
  return <SessionView key={sessionId} {...props} sessionId={sessionId} />;
}
function SessionView({
  reads,
  lifetime,
  sessionId,
  title,
  carrier,
}: {
  reads: (sessionId: string) => TrajectoryViewReads;
  lifetime: ContributionLifetime;
  sessionId: string;
  title: string;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const container = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<
    { type: "loading" } | { type: "ready" } | { type: "failure"; reason: string }
  >({ type: "loading" });
  useEffect(() => {
    const instance = lifetime.lifetime(`session:${sessionId}`);
    const signal = instance.signal;
    const started = container.current
      ? mountTrajectoryFrame({
          container: container.current,
          reads: reads(sessionId),
          carrier,
          signal,
          fail: (reason) => {
            if (!signal.aborted) setStatus({ type: "failure", reason });
          },
        }).then(
          (close) => {
            if (!signal.aborted) setStatus({ type: "ready" });
            return close;
          },
          (error) => {
            if (!signal.aborted)
              setStatus({
                type: "failure",
                reason:
                  error instanceof Error ? error.message : "The plugin page could not be opened.",
              });
            return undefined;
          },
        )
      : Promise.resolve(undefined);
    instance.cleanup(async () => {
      const close = await started;
      await close?.();
    });
    return () => {
      void instance[asyncDisposeSymbol]().catch(console.error);
    };
  }, [reads, sessionId, carrier, lifetime]);
  return (
    <section aria-label={title} {...stylex.props(styles.root)}>
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
    </section>
  );
}
