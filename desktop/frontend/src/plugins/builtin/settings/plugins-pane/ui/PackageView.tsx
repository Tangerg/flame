import { AgentActivityDisclosure, AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { useScheme } from "@/lib/appearance";
import { asyncDisposeSymbol } from "dougong";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { useEffect, useRef, useState, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewReads, PluginViewStatus } from "../application/pluginView";
import { mountPluginViewFrame } from "../adapters/pluginViewFrame";
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
export function PackageView<T>({
  reads,
  lifetime,
  title,
  carrier,
  fallback,
}: {
  reads: PluginViewReads<T>;
  lifetime: ContributionLifetime;
  title: string;
  carrier: PluginCarrier;
  fallback?: { label: string; content: ReactNode };
}) {
  const t = useT();
  const scheme = useScheme();
  const container = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<PluginViewStatus>({ type: "loading" });
  const [showRecorded, setShowRecorded] = useState(false);
  useEffect(() => {
    const instance = lifetime.lifetime("page");
    const signal = instance.signal;
    if (container.current)
      instance.cleanup(
        mountPluginViewFrame({
          container: container.current,
          reads,
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
  }, [reads, scheme, carrier, lifetime]);
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
      {fallback &&
        (status.type === "ready" ? (
          <AgentActivityDisclosure
            icon="file-text"
            label={fallback.label}
            shell="line"
            open={showRecorded}
            onToggle={() => setShowRecorded((value) => !value)}
          >
            {fallback.content}
          </AgentActivityDisclosure>
        ) : (
          fallback.content
        ))}
    </AgentWorkspaceView>
  );
}
