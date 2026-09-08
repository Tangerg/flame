import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { Badge, DataView, IconButton, TextButton } from "@/ui";
import type { Tone } from "@/lib/tone";
import {
  forgetApprovalRule,
  forgetApprovalRules,
  type ApprovalRuleSummary,
  useApprovalRuleConfigs,
} from "../application/approvalConfig";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { color, radius, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

// What the scope MEANS. `Tone`'s own contract says the application layer emits the
// vocabulary and the Badge picks the fill and the ink; a table of classes here is this file
// painting a second palette beside the one every other badge in the app uses.
const SCOPE_TONE: Record<ApprovalRuleSummary["scope"], Tone> = {
  session: "neutral",
  project: "accent",
  global: "warning",
};

const r = stylex.create({
  body: { marginTop: space.s3 },
  list: { display: "flex", flexDirection: "column", gap: space.s0_5 },
  rule: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
    transitionProperty: "background-color",
  },
  verdict: { flexShrink: 0, fontWeight: weight.medium },
  allow: { color: color.success },
  tool: { color: color.fg },
});

export function RulesRow() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data, isLoading, isError, error, refetch } = useApprovalRuleConfigs(sessionId);
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("approvals.error.forget"),
  });

  return (
    <div>
      <div {...stylex.props(ss.label, typeStep.uiMd)}>{t("approvals.rules")}</div>
      <div {...stylex.props(ss.hintSpaced, typeStep.uiMd)}>{t("approvals.rules.sub")}</div>
      <div {...stylex.props(r.body)}>
        <DataView
          items={data}
          isLoading={isLoading}
          isError={isError}
          onRetry={refetch}
          unsupported={
            isUnsupportedMethod(error)
              ? {
                  icon: "shield",
                  title: t("runtime.unsupported.title"),
                  sub: t("runtime.unsupported.sub"),
                }
              : undefined
          }
          empty={{
            icon: "check",
            title: t("approvals.rules.empty"),
            sub: t("approvals.rules.emptySub"),
          }}
        >
          {(rows) => (
            <div {...stylex.props(r.list)}>
              <div {...stylex.props(ss.end)}>
                <TextButton disabled={busy} onClick={() => run(() => forgetApprovalRules(rows))}>
                  {t("approvals.clearAll")}
                </TextButton>
              </div>
              {rows.map((rule) => (
                <div key={rule.id} {...stylex.props(r.rule)}>
                  <Badge tone={SCOPE_TONE[rule.scope]} face="mono">
                    {t(`approvals.scope.${rule.scope}`)}
                  </Badge>
                  <span
                    {...stylex.props(
                      r.verdict,
                      rule.decision === "deny" ? ss.negative : r.allow,
                      typeStep.uiSm,
                    )}
                  >
                    {rule.decision === "deny" ? t("approvals.deny") : t("approvals.allow")}
                  </span>
                  <span {...stylex.props(ss.fill, ss.truncate, ss.mono, r.tool, typeStep.uiMd)}>
                    {rule.tool}
                    {rule.subject ? (
                      <span {...stylex.props(ss.muted)}> · {rule.subject}</span>
                    ) : null}
                    {rule.dir ? <span {...stylex.props(ss.faint)}> — {rule.dir}</span> : null}
                  </span>
                  <IconButton
                    icon="x"
                    iconSize="sm"
                    size="xs"
                    quiet
                    className={stylex.props(ss.hold).className}
                    aria-label={t("approvals.forget", { tool: rule.tool })}
                    aria-busy={busy}
                    disabled={busy}
                    onClick={() => run(() => forgetApprovalRule(rule.id))}
                  />
                </div>
              ))}
            </div>
          )}
        </DataView>
      </div>
    </div>
  );
}
