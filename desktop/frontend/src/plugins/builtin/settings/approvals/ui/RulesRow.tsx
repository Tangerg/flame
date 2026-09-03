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
import { cn } from "@/lib/classNames";

// What the scope MEANS. `Tone`'s own contract says the application layer emits the
// vocabulary and the Badge picks the fill and the ink; a table of classes here is this file
// painting a second palette beside the one every other badge in the app uses.
const SCOPE_TONE: Record<ApprovalRuleSummary["scope"], Tone> = {
  session: "neutral",
  project: "accent",
  global: "warning",
};

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
      <div className="text-ui-md font-medium text-fg">{t("approvals.rules")}</div>
      <div className="mt-1 text-ui-md leading-body text-fg-muted">{t("approvals.rules.sub")}</div>
      <div className="mt-3">
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
            <div className="flex flex-col gap-0.5">
              <div className="flex justify-end">
                <TextButton disabled={busy} onClick={() => run(() => forgetApprovalRules(rows))}>
                  {t("approvals.clearAll")}
                </TextButton>
              </div>
              {rows.map((r) => (
                <div
                  key={r.id}
                  className="flex items-center gap-2 rounded-md px-2.5 py-2 transition-colors hover:bg-hover"
                >
                  <Badge tone={SCOPE_TONE[r.scope]} className="font-mono">
                    {t(`approvals.scope.${r.scope}`)}
                  </Badge>
                  <span
                    className={cn(
                      "shrink-0 text-ui-sm font-medium",
                      r.decision === "deny" ? "text-negative" : "text-success",
                    )}
                  >
                    {r.decision === "deny" ? t("approvals.deny") : t("approvals.allow")}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-mono text-ui-md text-fg">
                    {r.tool}
                    {r.subject ? <span className="text-fg-muted"> · {r.subject}</span> : null}
                    {r.dir ? <span className="text-fg-faint"> — {r.dir}</span> : null}
                  </span>
                  <IconButton
                    icon="x"
                    iconSize="sm"
                    size="xs"
                    quiet
                    className="shrink-0"
                    aria-label={t("approvals.forget", { tool: r.tool })}
                    aria-busy={busy}
                    disabled={busy}
                    onClick={() => run(() => forgetApprovalRule(r.id))}
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
