import { iconGlyph } from "@/ui/icons/index";
import { Button, EmptyState } from "@flame/ui";
import { useApprovalModeConfig } from "@/features/approvals/application/approvalConfig";
import { useT } from "@/lib/i18n/index";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { ModeRow } from "@/features/approvals/ui/ModeRow";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
export function ApprovalsPane() {
  const t = useT();
  const { data: approval, isError, refetch } = useApprovalModeConfig();
  if (isError) {
    return (
      <EmptyState
        icon={iconGlyph("alert")}
        tone="negative"
        title={t("approvals.unavailable")}
        sub={t("approvals.unavailable.sub")}
        action={
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t("common.retry")}
          </Button>
        }
      />
    );
  }
  return (
    <SettingsGroup>
      <ModeRow approval={approval} />
      <RulesRow />
    </SettingsGroup>
  );
}
