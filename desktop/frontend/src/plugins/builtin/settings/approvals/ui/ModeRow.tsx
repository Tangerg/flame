import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { ChoiceList, ChoiceOption, Icon, vocab } from "@/ui";
import { setApprovalMode } from "@/plugins/builtin/agent/public/approvalPolicy";
import { APPROVAL_MODES, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { useId, useState } from "react";
import {
  color,
  leading,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

type ApprovalModeIntent = {
  mode: ApprovalMode;
  settlement: "pending" | "accepted-awaiting-projection";
} | null;

const MODE_VALUES = APPROVAL_MODES.map((option) => option.value);

const m = stylex.create({
  // Holds the list's measure while the modes load, so the pane does not jump when they land.
  placeholder: {
    marginTop: space.s3,
    height: "184px",
    borderRadius: radius.lg,
    backgroundColor: surface.sunken,
  },
  list: { marginTop: space.s3 },
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
    <div>
      <div id={labelId} {...stylex.props(ss.label, typeStep.uiMd)}>
        {t("approvals.mode")}
      </div>
      <div {...stylex.props(ss.hintSpaced, typeStep.uiMd)}>{t("approvals.mode.sub")}</div>
      {mode === undefined ? (
        <div {...stylex.props(m.placeholder)} aria-hidden />
      ) : (
        <div {...stylex.props(m.list)}>
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
        </div>
      )}
    </div>
  );
}
