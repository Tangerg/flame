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

type ApprovalModeIntent = {
  mode: ApprovalMode;
  settlement: "pending" | "accepted-awaiting-projection";
} | null;

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
  const [intent, setIntent] = useState<ApprovalModeIntent>(null);
  const activeIntent =
    intent?.settlement === "accepted-awaiting-projection" && intent.mode === mode ? null : intent;
  const shown = activeIntent?.mode ?? mode;

  const onChange = async (next: ApprovalMode) => {
    if (activeIntent !== null || next === mode) return;
    setIntent({ mode: next, settlement: "pending" });
    try {
      const accepted = await setApprovalMode(next);
      setIntent((current) =>
        current?.mode === next
          ? { mode: accepted, settlement: "accepted-awaiting-projection" }
          : current,
      );
    } catch (err) {
      setIntent((current) => (current?.mode === next ? null : current));
      if (wasGenerationRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
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
        // The count comes from the list itself. It was a 184px slab, which is the same coupling
        // this option table refuses at its other end — a number that has to be re-measured when
        // a mode is added, a description wraps, or the reader picks a larger type size.
        <SkeletonList count={APPROVAL_MODES.length} label={t("common.loading")} />
      ) : (
        <ChoiceList
          multiple={false}
          value={shown === undefined ? [] : [shown]}
          values={MODE_VALUES}
          labelledBy={labelId}
          pending={activeIntent !== null}
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
              pending={activeIntent !== null}
              busy={o.value === activeIntent?.mode}
            >
              <span {...stylex.props(m.body)}>
                <span {...stylex.props(m.name, typeStep.uiMd)}>{t(o.labelKey)}</span>
                <span {...stylex.props(m.desc, typeStep.uiMd)}>{t(o.descKey)}</span>
              </span>
              {o.value === activeIntent?.mode && (
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
