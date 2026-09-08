import * as stylex from "@stylexjs/stylex";
import { EmptyState } from "@/ui";
import { useApprovalModeConfig } from "../application/approvalConfig";
import { useT } from "@/lib/i18n";
import { ModeRow } from "./ModeRow";
import { RulesRow } from "./RulesRow";
import { settingStyles as ss } from "../../kit/settingStyles";

export function ApprovalsPane() {
  const t = useT();
  const { data: mode, isError } = useApprovalModeConfig();
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
    <div {...stylex.props(ss.pane)}>
      <ModeRow mode={mode} />
      <RulesRow />
    </div>
  );
}
