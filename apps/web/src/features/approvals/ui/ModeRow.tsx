import { useAgent } from "@/features/agent/public/agent";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { ChoiceList, ChoiceOption } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SkeletonList } from "@/ui/index";
import { vocab } from "@flame/ui";
import {
  APPROVAL_MODE_LABEL_KEY,
  describeApprovalMode,
} from "@/features/agent/public/approvalPolicy";
import { setApprovalMode } from "@/features/agent/public/approvalPolicy";
import { type ApprovalMode, type ApprovalModeResult } from "@/features/agent/public/approvalPolicy";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useT } from "@/lib/i18n/index";
import { useId, useState } from "react";
import { color, leading, motion, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { SettingRow } from "@/ui/settings/SettingRow";
const m = stylex.create({
  body: { display: "flex", minWidth: 0, flex: 1, flexDirection: "column", gap: space.s0_5 },
  name: { color: color.fg, fontWeight: weight.medium },
  desc: { color: color.fgMuted, lineHeight: leading.body },
  spin: { animation: motion.spin },
});

export function ModeRow({ approval }: { approval: ApprovalModeResult | undefined }) {
  const agentAccess = useAgent();

  const t = useT();
  const mode = approval?.mode;
  const labelId = useId();
  const [pending, setPending] = useState<ApprovalMode | null>(null);
  const shown = pending ?? mode;
  const modeAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("approvals.error.mode"),
  });

  const onChange = (next: ApprovalMode) => {
    if (pending !== null || next === mode) return;
    setPending(next);
    modeAction.run(() => setApprovalMode(agentAccess, next).finally(() => setPending(null)));
  };
  return (
    <SettingRow
      label={t("approvals.mode")}
      labelId={labelId}
      sub={t("approvals.mode.sub")}
      align="stacked"
    >
      {approval === undefined ? (
        <SkeletonList
          count={Object.keys(APPROVAL_MODE_LABEL_KEY).length}
          label={t("common.loading")}
        />
      ) : (
        <ChoiceList
          multiple={false}
          value={shown === undefined ? [] : [shown]}
          values={approval.modes.map((policy) => policy.mode)}
          labelledBy={labelId}
          pending={pending !== null}
          onValueChange={([next]) => {
            if (next !== undefined) onChange(next as ApprovalMode);
          }}
        >
          {approval.modes.map((policy) => (
            <ChoiceOption
              key={policy.mode}
              multiple={false}
              value={policy.mode}
              selected={policy.mode === shown}
              label={t(APPROVAL_MODE_LABEL_KEY[policy.mode])}
              description={describeApprovalMode(policy, t)}
              pending={pending !== null}
              busy={policy.mode === pending}
            >
              <span {...stylex.props(m.body)}>
                <span {...stylex.props(m.name, typeStep.uiMd)}>
                  {t(APPROVAL_MODE_LABEL_KEY[policy.mode])}
                </span>
                <span {...stylex.props(m.desc, typeStep.uiMd)}>
                  {describeApprovalMode(policy, t)}
                </span>
              </span>
              {policy.mode === pending && (
                <Icon
                  name="loop"
                  size="sm"
                  className={stylex.props(vocab.hold, m.spin, vocab.accent).className}
                />
              )}
            </ChoiceOption>
          ))}
        </ChoiceList>
      )}
    </SettingRow>
  );
}
