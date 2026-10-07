import { EmptyState } from "@/ui";
import { useApprovalModeConfig } from "../application/approvalConfig";
import { useT } from "@/lib/i18n";
import { SettingsGroup } from "../../kit";
import { ModeRow } from "./ModeRow";
import { RulesRow } from "./RulesRow";

export function ApprovalsPane() {
  const t = useT();
  const { data: approval, isError } = useApprovalModeConfig();
  if (isError) {
    return (
      <EmptyState
        icon="shield"
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
