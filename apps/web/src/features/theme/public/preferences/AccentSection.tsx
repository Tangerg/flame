import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n/index";
import { ACCENT } from "@/features/theme/public/extensionPoints";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { solidInk, solidFill } from "@/features/theme/kit/scale";
import { useAppearance, useAppearanceEdit } from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
import { ColorPickerInput } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Pressable } from "@/ui/index";
import { corner, motion, space, surface } from "@flame/ui/tokens.stylex";
const RAINBOW_HINT =
  "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

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
    transitionTimingFunction: motion.easeState,
    scale: { default: null, ":active": "var(--press-scale)" },
  },
  noPad: { padding: 0 },
  chrome: {
    height: space.s5,
    width: space.s5,
    borderWidth: "2px",
    borderStyle: "solid",
    borderColor: "transparent",
    backgroundClip: "padding-box",
    transitionProperty: "scale, box-shadow",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
    scale: "var(--accent-lift, 1)",
  },
  chromeOn: { borderColor: surface.surface, boxShadow: "var(--shadow-swatch-selected)" },
  mark: {
    pointerEvents: "none",
    position: "absolute",
    transitionProperty: "opacity, scale",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
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

function SelectionMark({ selected, fill }: { selected: boolean; fill: string }) {
  return (
    <span
      aria-hidden
      data-slot="accent-selection-mark"
      {...stylex.props(a.mark, selected ? a.markOn : a.markOff)}
      style={{ color: solidInk(fill) }}
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
  return (
    <span
      data-focus-proxy=""
      title={label}
      className={stylex.props(a.target, corner.pill).className}
    >
      <span
        aria-hidden
        className={stylex.props(a.chrome, corner.pill, isActive && a.chromeOn).className}
        style={{ background: isActive ? value : RAINBOW_HINT }}
      />
      <SelectionMark selected={isActive} fill={value} />
      <ColorPickerInput aria-label={label} value={value} onValueChange={onChange} />
    </span>
  );
}

export function AccentSection() {
  const t = useT();
  const accents = useExtensionPoint(ACCENT);
  const accent = useAppearance((s) => s.accent);
  const { setAccent } = useAppearanceEdit();
  const isCustom = !accents.some((a) => a.color === accent);

  return (
    <SettingRow label={t("settings.accent")}>
      <div {...stylex.props(a.swatchRow)}>
        {accents.map((swatch) => {
          const fill = solidFill(swatch.color);
          return (
            <Pressable
              key={swatch.id}
              type="button"
              onClick={() => setAccent(swatch.color)}
              title={`${t("settings.accent")}: ${swatch.label}`}
              aria-label={`${t("settings.accent")}: ${swatch.label}`}
              aria-pressed={accent === swatch.color}
              className={stylex.props(a.target, a.noPad, corner.pill).className}
            >
              <span
                aria-hidden
                className={
                  stylex.props(a.chrome, corner.pill, accent === swatch.color && a.chromeOn)
                    .className
                }
                style={{ background: fill }}
              />
              <SelectionMark selected={accent === swatch.color} fill={fill} />
            </Pressable>
          );
        })}
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
