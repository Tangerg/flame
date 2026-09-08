import * as stylex from "@stylexjs/stylex";
import { EmptyState, gap, vocab } from "@/ui";
import { useApprovalModeConfig } from "../application/approvalConfig";
import { useT } from "@/lib/i18n";
import { ModeRow } from "./ModeRow";
import { RulesRow } from "./RulesRow";

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
    <div {...stylex.props(vocab.column, gap.s6)}>
      <ModeRow mode={mode} />
      <RulesRow />
    </div>
  );
}
