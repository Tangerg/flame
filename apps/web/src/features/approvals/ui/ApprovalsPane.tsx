import { DataError } from "@/ui/index";
import { useApprovalModeConfig } from "@/features/approvals/application/approvalConfig";
import { useT } from "@/lib/i18n/index";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { ModeRow } from "@/features/approvals/ui/ModeRow";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
export function ApprovalsPane() {
  const t = useT();
  const { data: approval, isError, refetch } = useApprovalModeConfig();
  return (
    <SettingsGroup>
      {isError ? (
        <DataError
          onRetry={() => refetch()}
          title={t("approvals.unavailable")}
          sub={t("approvals.unavailable.sub")}
        />
      ) : (
        <ModeRow approval={approval} />
      )}
      <RulesRow />
    </SettingsGroup>
  );
}
