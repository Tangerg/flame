import * as stylex from "@stylexjs/stylex";
import { useId } from "react";
import { useT } from "@/lib/i18n";
import { ACCENT, useExtensionPoint } from "@/plugins/sdk";
import { useAccentPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { ColorPickerInput, Icon, Pressable } from "@/ui";
import { corner, motion, space, surface } from "@/styles/tokens.stylex";

const RAINBOW_HINT =
  "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

// The target publishes the lift and the chrome reads it. StyleX has no ancestor selector, so
// `group-hover/accent:scale-105` becomes a channel — which also states plainly that the target
// owns "the pointer is on me" and the chrome owns what that looks like.
const a = stylex.create({
  target: {
    "--accent-lift": { default: "1", ":hover": "1.05" },
    position: "relative",
    display: "inline-grid",
    height: space.s7,
    width: space.s7,
    placeItems: "center",
    backgroundColor: { default: "transparent", ":hover": surface.hover },
    transitionProperty: "background-color, scale",
    transitionDuration: motion.fast,
    scale: { default: null, ":active": "var(--press-scale)" },
  },
  noPad: { padding: 0 },
  chrome: {
    height: space.s5,
    width: space.s5,
    borderWidth: "2px",
    borderStyle: "solid",
    borderColor: "transparent",
    // Keeps the fill out from under the ring, so a selected swatch shows the surface between.
    backgroundClip: "padding-box",
    transitionProperty: "scale, box-shadow",
    transitionDuration: motion.fast,
    scale: "var(--accent-lift, 1)",
  },
  chromeOn: { borderColor: surface.surface, boxShadow: "var(--shadow-swatch-selected)" },
  mark: {
    pointerEvents: "none",
    position: "absolute",
    color: surface.surface,
    transitionProperty: "opacity, scale",
    transitionDuration: motion.fast,
  },
  markOn: { scale: 1, opacity: 1 },
  markOff: { scale: 0.75, opacity: 0 },
  swatchRow: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "flex-start",
    gap: space.s2_5,
  },
});

function SelectionMark({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden
      data-slot="accent-selection-mark"
      {...stylex.props(a.mark, selected ? a.markOn : a.markOff)}
    >
      <Icon name="check" size="xs" />
    </span>
  );
}

function CustomAccentPicker({
  value,
  isActive,
  onChange,
  label,
}: {
  value: string;
  isActive: boolean;
  onChange: (hex: string) => void;
  label: string;
}) {
  const inputId = useId();
  return (
    <label
      htmlFor={inputId}
      title={label}
      aria-label={label}
      className={stylex.props(a.target, corner.pill).className}
    >
      <span
        aria-hidden
        className={stylex.props(a.chrome, corner.pill, isActive && a.chromeOn).className}
        style={{ background: isActive ? value : RAINBOW_HINT }}
      />
      <SelectionMark selected={isActive} />
      <ColorPickerInput
        id={inputId}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  );
}

export function AccentSection() {
  const t = useT();
  const accents = useExtensionPoint(ACCENT);
  const { accent, setAccent, scheme } = useAccentPreference();
  const light = scheme === "light";

  const isCustom = !accents.some((a) => a.dark === accent);

  return (
    <SettingRow label={t("settings.accent")} sub={t("settings.accent.sub")}>
      <div {...stylex.props(a.swatchRow)}>
        {accents.map((swatch) => (
          <Pressable
            key={swatch.id}
            type="button"
            onClick={() => setAccent(swatch.dark)}
            title={`${t("settings.accent")}: ${swatch.label}`}
            aria-label={`${t("settings.accent")}: ${swatch.label}`}
            aria-pressed={accent === swatch.dark}
            className={stylex.props(a.target, a.noPad, corner.pill).className}
          >
            <span
              aria-hidden
              className={
                stylex.props(a.chrome, corner.pill, accent === swatch.dark && a.chromeOn).className
              }
              style={{ background: light ? (swatch.light ?? swatch.dark) : swatch.dark }}
            />
            <SelectionMark selected={accent === swatch.dark} />
          </Pressable>
        ))}
        <CustomAccentPicker
          value={accent}
          isActive={isCustom}
          onChange={setAccent}
          label={t("settings.accent.custom")}
        />
      </div>
    </SettingRow>
  );
}
