import { iconGlyph } from "@/ui/icons/index";
import { EmptyState } from "@flame/ui";
import { useApprovalModeConfig } from "@/features/approvals/application/approvalConfig";
import { useT } from "@/lib/i18n/index";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { ModeRow } from "@/features/approvals/ui/ModeRow";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
export function ApprovalsPane() {
  const t = useT();
  const { data: approval, isError } = useApprovalModeConfig();
  if (isError) {
    return (
      <EmptyState
        icon={iconGlyph("shield")}
        title={t("approvals.unavailable")}
        sub={t("approvals.unavailable.sub")}
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
