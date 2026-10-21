import { iconGlyph } from "@/ui/icons/index";
import { toolSourceLabel } from "@/lib/toolSource";
import { type ApprovalRuleSummary } from "@/features/agent/public/approvalPolicy";
import * as stylex from "@stylexjs/stylex";
import { useId } from "react";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { DataView } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Badge, IconButton, vocab } from "@flame/ui";
import { type Tone } from "@flame/ui/badge";
import {
  forgetApprovalRule,
  forgetApprovalRules,
  useApprovalRuleConfigs,
} from "@/features/approvals/application/approvalConfig";
import { useActiveSessionId } from "@/features/agent/public/session";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useT } from "@/lib/i18n/index";
import {
  color,
  face,
  layout,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@flame/ui/tokens.stylex";
import { SettingRow } from "@/ui/settings/SettingRow";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const SCOPE_TONE: Record<ApprovalRuleSummary["scope"], Tone> = {
  session: "neutral",
  project: "accent",
  global: "warning",
};

const r = stylex.create({
  list: { display: "flex", flexDirection: "column", gap: space.s0_5 },
  rule: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "start",
    gap: layout.controlGap,
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
  },
  copy: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    gap: layout.labelGap,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  identity: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  details: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    gap: layout.labelGap,
    minWidth: 0,
    whiteSpace: "pre-wrap",
  },
  verdict: { flexShrink: 0, fontWeight: weight.medium },
  allow: { color: color.success },
  tool: { color: color.fg, minWidth: 0, maxWidth: "100%" },
});

export function RulesRow() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data, isLoading, error, refetch } = useApprovalRuleConfigs(sessionId);
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("approvals.error.forget"),
  });
  const descriptionPrefix = useId();

  return (
    <SettingRow label={t("approvals.rules")} sub={t("approvals.rules.sub")} align="stacked">
      <DataView
        items={data}
        isLoading={isLoading}
        failure={error}
        onRetry={refetch}
        unsupported={{ icon: "shield" }}
        empty={{
          icon: "check",
          title: t("approvals.rules.empty"),
          sub: t("approvals.rules.emptySub"),
        }}
      >
        {(rows) => (
          <div {...stylex.props(r.list)}>
            <div {...stylex.props(ss.end)}>
              <TextButton pending={busy} onClick={() => run(() => forgetApprovalRules(rows))}>
                {t("approvals.clearAll")}
              </TextButton>
            </div>
            {rows.map((rule, index) => {
              const descriptionId = `${descriptionPrefix}-${index}`;
              return (
                <div key={rule.id} {...stylex.props(r.rule)}>
                  <div id={descriptionId} {...stylex.props(r.copy)}>
                    <div {...stylex.props(r.identity)}>
                      <Badge tone={SCOPE_TONE[rule.scope]} face="mono">
                        {t(`approvals.scope.${rule.scope}`)}
                      </Badge>
                      <span
                        {...stylex.props(
                          r.verdict,
                          rule.decision === "deny" ? vocab.negative : r.allow,
                          typeStep.uiSm,
                        )}
                      >
                        {rule.decision === "deny" ? t("approvals.deny") : t("approvals.allow")}
                      </span>
                      <span {...stylex.props(r.tool, typeStep.uiMd, face.mono)}>
                        {rule.modelName}
                      </span>
                      {rule.stale ? <Badge tone="warning">{t("approvals.stale")}</Badge> : null}
                    </div>
                    <div {...stylex.props(r.details, vocab.muted, typeStep.uiSm, face.mono)}>
                      <span>
                        {rule.tool.type === "builtIn"
                          ? t("approvals.builtIn")
                          : toolSourceLabel(rule.tool)}
                      </span>
                      <span>
                        {t(`approvals.subject.${rule.subject.type}`)}
                        {rule.subject.type !== "all" ? `: ${rule.subject.value}` : null}
                      </span>
                      {rule.dir ? <span>{rule.dir}</span> : null}
                    </div>
                  </div>
                  <IconButton
                    icon={iconGlyph("x")}
                    size="xs"
                    quiet
                    styles={vocab.hold}
                    aria-label={t("approvals.forget", { tool: rule.modelName })}
                    aria-describedby={descriptionId}
                    aria-busy={busy}
                    pending={busy}
                    onClick={() => run(() => forgetApprovalRule(rule.id))}
                  />
                </div>
              );
            })}
          </div>
        )}
      </DataView>
    </SettingRow>
  );
}
