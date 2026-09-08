import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { Badge, DataView, EmptyState, gap, Icon, Surface, Switch, Tag, vocab } from "@/ui";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/rpcErrors";
import type { HookReadModel } from "../application/hookConfig";
import { useHookConfigs } from "../application/hookConfig";
import { setHookTrust } from "../application/hookTrust";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { useRef, useState } from "react";
import { color, face, leading, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const hp = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s3,
  },
  // A hook the trust gate has not admitted. It is not disabled — the pane is telling you it
  // exists and is being ignored — so it steps back without joining the disabled step.
  inactive: { opacity: "var(--state-receded)" },
  injected: { fontStyle: "italic" },
  scope: { color: color.fgFaint, fontWeight: weight.medium },
  sub: { marginTop: space.s0_5, color: color.fgMuted, lineHeight: leading.body },
});

function HookRow({ h }: { h: HookReadModel }) {
  const t = useT();
  return (
    <div {...stylex.props(hp.row, ss.hoverRow, !h.active && hp.inactive)}>
      <Icon
        name={h.scope === "global" ? "globe" : "folder"}
        size="sm"
        className={stylex.props(vocab.faint).className}
      />
      <div {...stylex.props(vocab.line, vocab.min)}>
        <Tag>{h.event}</Tag>
        {h.matcher && (
          <span
            {...stylex.props(vocab.hold, vocab.accent, typeStep.uiSm, face.mono)}
            title={t("hooks.matcher")}
          >
            {h.matcher}
          </span>
        )}
        <span
          {...stylex.props(vocab.fill, ss.monoName, typeStep.uiMd)}
          title={h.command || h.inject || h.source}
        >
          {h.command ? (
            h.command
          ) : (
            <span {...stylex.props(vocab.muted, hp.injected)}>{h.inject}</span>
          )}
        </span>
      </div>
      {!h.active ? (
        <Badge tone="warning" title={t("hooks.inactive.hint")}>
          {t("hooks.inactive")}
        </Badge>
      ) : h.inject ? (
        <span {...stylex.props(vocab.hold, hp.scope, typeStep.uiXs)}>{t("hooks.kind.inject")}</span>
      ) : null}
    </div>
  );
}

export function HooksPane() {
  const t = useT();
  const [trusting, setTrusting] = useState(false);
  const trustingRef = useRef(false);
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, error, refetch } = useHookConfigs(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );

  if (isError && isUnsupportedMethod(error)) {
    return (
      <EmptyState
        icon="lightning"
        title={t("hooks.unavailable")}
        sub={t("hooks.unavailable.sub")}
      />
    );
  }

  const projectRoot = data?.projectRoot;

  const onTrust = async (trusted: boolean) => {
    if (!projectRoot || trustingRef.current) return;
    trustingRef.current = true;
    setTrusting(true);
    try {
      await setHookTrust(projectRoot, trusted);
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("hooks.error.trust"));
    } finally {
      trustingRef.current = false;
      setTrusting(false);
    }
  };

  return (
    <div {...stylex.props(vocab.column, gap.s4)}>
      <p {...stylex.props(ss.intro, typeStep.uiMd)}>{t("hooks.intro")}</p>

      {projectRoot && data?.hasProjectHooks && (
        <Surface className={stylex.props(ss.split).className}>
          <div {...stylex.props(vocab.min)}>
            <div {...stylex.props(ss.label, typeStep.uiMd)}>{t("hooks.trust")}</div>
            <div {...stylex.props(hp.sub, typeStep.uiMd)}>{t("hooks.trust.sub")}</div>
            <div
              {...stylex.props(
                vocab.afterLine,
                vocab.truncate,
                vocab.faint,
                typeStep.uiSm,
                face.mono,
              )}
              title={projectRoot}
            >
              {projectRoot}
            </div>
          </div>
          <Switch
            checked={data?.projectTrusted ?? false}
            disabled={trusting}
            onCheckedChange={(v) => void onTrust(v)}
            ariaLabel={t("hooks.trust.aria")}
          />
        </Surface>
      )}

      <DataView
        items={data?.hooks}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{ icon: "lightning", title: t("hooks.empty"), sub: t("hooks.empty.sub") }}
      >
        {(rows) => (
          <div {...stylex.props(vocab.column, gap.s0_5)}>
            {rows.map((h, i) => (
              <HookRow key={`${h.source}:${h.event}:${i}`} h={h} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
