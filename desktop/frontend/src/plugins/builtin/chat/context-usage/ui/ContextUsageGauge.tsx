import * as stylex from "@stylexjs/stylex";
import { Gauge, Pressable, RichTooltip, vocab } from "@/ui";
import { fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { contextUsageReadout } from "../application/contextUsageReadout";
import { color, motion, radius, space, surface } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

const cu = stylex.create({
  // Pulls back into the bar's own inset: the gauge is a glyph, not a control with a box.
  trigger: {
    marginInline: "calc(var(--spacing) * -1.5)",
    display: "inline-flex",
    height: space.s7,
    width: space.s7,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: radius.sm,
    backgroundColor: { default: null, ":hover": surface.hover },
    color: { default: color.fgMuted, ":hover": color.fg },
    transitionProperty: "background-color, color",
    transitionDuration: motion.color,
  },
  panel: { width: "calc(var(--spacing) * 38)" },
});

export function ContextUsageGauge() {
  const t = useT();
  const currentRun = useCurrentRootMaterial();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const { data: models = [] } = useModels();
  const servedSelection =
    currentRun.modelSelection ?? sessions?.find((session) => session.id === activeSessionId);
  const servedModel = models.find(
    (model) => model.provider === servedSelection?.provider && model.id === servedSelection?.model,
  );
  const readout = contextUsageReadout(
    currentRun.contextTokens ?? undefined,
    servedModel?.tokenLimits?.contextWindow,
  );
  if (!readout) return null;

  const label = t("context.usage.aria", { percent: readout.percent });
  const trigger = (
    <Pressable aria-label={label} className={stylex.props(cu.trigger).className}>
      <Gauge value={readout.ratio} label={t("context.usage.aria", { percent: readout.percent })} />
    </Pressable>
  );

  return (
    <RichTooltip
      trigger={trigger}
      side="top"
      sideOffset={4}
      className={stylex.props(cu.panel).className}
    >
      <div {...stylex.props(vocab.stackHairline, ct.centre)}>
        <span {...stylex.props(vocab.muted)}>{t("context.usage.label")}</span>
        <span {...stylex.props(readout.percent >= 50 && vocab.muted)}>
          {t(readout.percent >= 50 ? "context.usage.statusFull" : "context.usage.statusLeft", {
            percent: readout.percent,
            remaining: 100 - readout.percent,
          })}
        </span>
        <span>
          {t("context.usage.tooltip", {
            used: fmtTokens(readout.usedTokens),
            window: fmtTokens(readout.windowTokens),
          })}
        </span>
      </div>
    </RichTooltip>
  );
}
