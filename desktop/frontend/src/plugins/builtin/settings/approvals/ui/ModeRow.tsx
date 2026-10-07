import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { ChoiceList, ChoiceOption, Icon, SkeletonList, vocab } from "@/ui";
import { setApprovalMode } from "@/plugins/builtin/agent/public/approvalPolicy";
import { APPROVAL_MODES, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { useId, useState } from "react";
import { color, leading, motion, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { SettingRow } from "../../kit";

const MODE_VALUES = APPROVAL_MODES.map((option) => option.value);

const m = stylex.create({
  body: { display: "flex", minWidth: 0, flex: 1, flexDirection: "column", gap: space.s0_5 },
  name: { color: color.fg, fontWeight: weight.medium },
  desc: { color: color.fgMuted, lineHeight: leading.body },
  spin: { animation: motion.spin },
});

export function ModeRow({ mode }: { mode: ApprovalMode | undefined }) {
  const t = useT();
  const labelId = useId();
  const [pending, setPending] = useState<ApprovalMode | null>(null);
  const shown = pending ?? mode;

  const onChange = async (next: ApprovalMode) => {
    if (pending !== null || next === mode) return;
    setPending(next);
    try {
      await setApprovalMode(next);
    } catch (err) {
      if (!wasGenerationRetired(err)) notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    } finally {
      setPending(null);
    }
  };
  return (
    <SettingRow
      label={t("approvals.mode")}
      labelId={labelId}
      sub={t("approvals.mode.sub")}
      align="stacked"
    >
      {mode === undefined ? (
        <SkeletonList count={APPROVAL_MODES.length} label={t("common.loading")} />
      ) : (
        <ChoiceList
          multiple={false}
          value={shown === undefined ? [] : [shown]}
          values={MODE_VALUES}
          labelledBy={labelId}
          pending={pending !== null}
          onValueChange={([next]) => {
            if (next !== undefined) void onChange(next as ApprovalMode);
          }}
        >
          {APPROVAL_MODES.map((o) => (
            <ChoiceOption
              key={o.value}
              multiple={false}
              value={o.value}
              selected={o.value === shown}
              label={t(o.labelKey)}
              description={t(o.descKey)}
              pending={pending !== null}
              busy={o.value === pending}
            >
              <span {...stylex.props(m.body)}>
                <span {...stylex.props(m.name, typeStep.uiMd)}>{t(o.labelKey)}</span>
                <span {...stylex.props(m.desc, typeStep.uiMd)}>{t(o.descKey)}</span>
              </span>
              {o.value === pending && (
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
