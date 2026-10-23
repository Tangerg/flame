import { iconGlyph } from "@/ui/icons/index";
import { DataError } from "@/ui/index";
import { EmptyState } from "@flame/ui/empty-state";
import { useApprovalModeConfig } from "@/features/approvals/application/approvalConfig";
import { useT } from "@/lib/i18n/index";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { ModeRow } from "@/features/approvals/ui/ModeRow";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
export function ApprovalsPane() {
  const t = useT();
  const { data: approval, error, isError, refetch } = useApprovalModeConfig();
  return (
    <SettingsGroup>
      {isError && isUnsupportedMethod(error) ? (
        <EmptyState
          icon={iconGlyph("shield")}
          tone="warning"
          title={t("approvals.unavailable")}
          sub={t("approvals.unavailable.sub")}
        />
      ) : isError ? (
        <DataError onRetry={() => refetch()} title={t("approvals.error.loadMode")} />
      ) : (
        <ModeRow approval={approval} />
      )}
      <RulesRow />
    </SettingsGroup>
  );
}
