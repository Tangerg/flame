import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { Icon, Pressable, vocab } from "@/ui";
import { setApprovalMode } from "@/plugins/builtin/agent/public/approvalPolicy";
import { APPROVAL_MODES, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { useState } from "react";
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

const m = stylex.create({
  // Holds the list's measure while the modes load, so the pane does not jump when they land.
  placeholder: {
    marginTop: space.s3,
    height: "184px",
    borderRadius: radius.lg,
    backgroundColor: surface.sunken,
  },
  list: { marginTop: space.s3, display: "flex", flexDirection: "column", gap: space.s0_5 },
  option: {
    display: "flex",
    alignItems: "center",
    gap: space.s3,
    borderRadius: radius.card,
    paddingInline: space.s3,
    paddingBlock: space.s3,
    textAlign: "left",
    transitionProperty: "background-color",
  },
  optionOn: { backgroundColor: surface.accentWash },
  optionOff: { backgroundColor: { default: null, ":hover": surface.hover } },
  nameOn: { color: color.accent, fontWeight: weight.medium },
  nameOff: { color: color.fg },
  desc: { marginTop: space.s0_5, color: color.fgMuted, lineHeight: leading.body },
  spin: { animation: motion.spin },
});

export function ModeRow({ mode }: { mode: ApprovalMode | undefined }) {
  const t = useT();
  const [intent, setIntent] = useState<ApprovalModeIntent>(null);
  const activeIntent =
    intent?.settlement === "accepted-awaiting-projection" && intent.mode === mode ? null : intent;

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
      <div {...stylex.props(ss.label, typeStep.uiMd)}>{t("approvals.mode")}</div>
      <div {...stylex.props(ss.hintSpaced, typeStep.uiMd)}>{t("approvals.mode.sub")}</div>
      {mode === undefined ? (
        <div {...stylex.props(m.placeholder)} aria-hidden />
      ) : (
        <div {...stylex.props(m.list)}>
          {APPROVAL_MODES.map((o) => {
            const selected = o.value === (activeIntent?.mode ?? mode);
            const saving = o.value === activeIntent?.mode;
            return (
              <Pressable
                key={o.value}
                type="button"
                aria-pressed={selected}
                aria-label={t(o.labelKey)}
                aria-busy={saving || undefined}
                disabled={activeIntent !== null}
                onClick={() => void onChange(o.value)}
                className={stylex.props(m.option, selected ? m.optionOn : m.optionOff).className}
              >
                <div {...stylex.props(vocab.fill)}>
                  <div {...stylex.props(selected ? m.nameOn : m.nameOff, typeStep.uiMd)}>
                    {t(o.labelKey)}
                  </div>
                  <div {...stylex.props(m.desc, typeStep.uiMd)}>{t(o.descKey)}</div>
                </div>
                {saving ? (
                  <Icon
                    name="loop"
                    size="sm"
                    className={stylex.props(vocab.hold, m.spin, vocab.accent).className}
                  />
                ) : (
                  selected && (
                    <Icon
                      name="check"
                      size="md"
                      className={stylex.props(vocab.hold, vocab.accent).className}
                    />
                  )
                )}
              </Pressable>
            );
          })}
        </div>
      )}
    </div>
  );
}
