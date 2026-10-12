import { Button } from "@flame/ui/button";
import { reportViewFailure } from "../adapters/pluginViewDiagnostics";
import { AgentActivityDisclosure, AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { captureTheme, type ThemeProjection } from "@flame/ui/theme";
import { useTokenRevision, useUiFontSize } from "@/lib/appearance";
import { useScheme } from "@/lib/appearance";
import { asyncDisposeSymbol } from "dougong";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewReads, PluginViewStatus } from "../application/pluginView";
import { openPluginViewFrame } from "../adapters/pluginViewFrame";
import { space, color, type as typeStep } from "@flame/ui/tokens.stylex";

const styles = stylex.create({
  root: { width: "100%", height: "100%" },
  identity: {
    paddingInline: space.s4,
    paddingBlock: space.s2,
    color: color.fgMuted,
    flexShrink: 0,
  },
  carrier: { flex: 1, minHeight: 0, width: "100%" },
  status: {
    padding: space.s4,
    color: color.fgMuted,
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: space.s2,
  },
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
  const revision = useTokenRevision();
  const fontSize = useUiFontSize();
  const appearance = useRef<ThemeProjection | undefined>(undefined);
  const frame = useRef<ReturnType<typeof openPluginViewFrame<T>> | undefined>(undefined);
  useLayoutEffect(() => {
    appearance.current = captureTheme(document.documentElement, scheme, fontSize);
    frame.current?.updateTheme(appearance.current);
  }, [scheme, revision, fontSize]);
  const container = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<PluginViewStatus>({ type: "loading" });
  const [showRecorded, setShowRecorded] = useState(false);
  const [handoff, setHandoff] = useState<"idle" | "pending" | "failed">("idle");
  const retry = useRef<(() => void) | undefined>(undefined);
  const retirement = useRef<Promise<void>>(Promise.resolve());
  useEffect(() => {
    const predecessor = retirement.current;
    const owner = lifetime.lifetime("view");
    let page: ContributionLifetime | undefined;
    let starting: Promise<void> | undefined;
    const start = () => {
      if (starting || owner.signal.aborted) return;
      setHandoff("pending");
      const job = (async () => {
        await predecessor;
        await page?.[asyncDisposeSymbol]();
        if (owner.signal.aborted || !container.current) return;
        const instance = owner.lifetime("page");
        page = instance;
        setHandoff("idle");
        const controller = openPluginViewFrame({
          container: container.current,
          reads,
          carrier,
          signal: instance.signal,
          theme: appearance.current!,
          status: (value) => {
            if (!instance.signal.aborted) setStatus(value);
          },
        });
        frame.current = controller;
        instance.cleanup(controller.close);
      })()
        .catch((error) => {
          reportViewFailure(reads.binding, "retirement", error);
          if (!owner.signal.aborted) setHandoff("failed");
        })
        .finally(() => {
          if (starting === job) starting = undefined;
        });
      starting = job;
    };
    owner.cleanup(async () => {
      const results = await Promise.allSettled([starting, page?.[asyncDisposeSymbol]()]);
      const errors = results.flatMap((result) =>
        result.status === "rejected" ? [result.reason] : [],
      );
      if (errors.length) throw new AggregateError(errors, "The plugin view could not be retired.");
    });
    retry.current = start;
    start();
    return () => {
      frame.current = undefined;
      retry.current = undefined;
      retirement.current = owner[asyncDisposeSymbol]();
      void retirement.current.catch((error) =>
        reportViewFailure(reads.binding, "retirement", error),
      );
    };
  }, [reads, carrier, lifetime]);
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
      {handoff === "failed" && (
        <p role="alert" {...stylex.props(styles.status)}>
          {t("packages.view.retirementFailed")}
        </p>
      )}
      {status.type === "failure" && handoff !== "failed" && (
        <div {...stylex.props(styles.status)}>
          <p role="alert">{status.reason}</p>
          {(status.stage === "resource" || status.stage === "initial") && (
            <Button
              size="sm"
              variant="soft"
              disabled={handoff === "pending"}
              onClick={() => retry.current?.()}
            >
              {t("common.retry")}
            </Button>
          )}
        </div>
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
