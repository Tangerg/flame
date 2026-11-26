import { iconGlyph } from "@/ui/icons/index";
import { DataError } from "@/ui/index";
import { EmptyState } from "@flame/ui/empty-state";
import { useApprovalModeConfig } from "@/features/approvals/application/approvalConfig";
import { useT } from "@/lib/i18n/index";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { ModeSection } from "@/features/approvals/ui/ModeSection";
import { RulesSection } from "@/features/approvals/ui/RulesSection";
export function ApprovalsPane() {
  const t = useT();
  const { data: approval, error, isError, refetch } = useApprovalModeConfig();
  return (
    <>
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
        <ModeSection approval={approval} />
      )}
      <RulesSection />
    </>
  );
}
