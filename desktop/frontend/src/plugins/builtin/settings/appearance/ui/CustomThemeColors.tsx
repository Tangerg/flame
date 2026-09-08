import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { useCustomThemePreference } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { ColorPickerInput, vocab } from "@/ui";
import { color, corner, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const a = stylex.create({
  swatchLine: { position: "relative", display: "inline-flex", alignItems: "center", gap: space.s2 },
  hex: { fontFamily: "var(--font-mono)", textTransform: "uppercase", color: color.fg },
  // `bg-clip-padding` keeps the fill out from under the hairline, so a light colour does not
  // bleed through the border it is meant to sit inside.
  chip: {
    // A round chip, so the corner comes from the bundle that carries the shape with it.
    height: "calc(var(--spacing) * 4.5)",
    width: "calc(var(--spacing) * 4.5)",
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundClip: "padding-box",
  },
  colorGrid: { display: "grid", maxWidth: "300px", gap: space.s2 },
});

function ColorRow({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (hex: string) => void;
}) {
  return (
    <label {...stylex.props(ss.sunkenRow)}>
      <span {...stylex.props(vocab.muted, typeStep.uiMd)}>{label}</span>
      <span {...stylex.props(a.swatchLine)}>
        <span {...stylex.props(a.hex, typeStep.uiMd)}>{value}</span>
        <span
          className={stylex.props(a.chip, corner.pill).className}
          style={{ background: value }}
        />
        <ColorPickerInput
          aria-label={label}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      </span>
    </label>
  );
}

export function CustomThemeColors() {
  const t = useT();
  const { theme, customTheme, setCustomTheme } = useCustomThemePreference();

  if (theme !== "custom") return null;

  return (
    <SettingRow
      label={t("settings.customColors")}
      sub={t("settings.customColors.sub")}
      align="start"
    >
      <div {...stylex.props(a.colorGrid)}>
        <ColorRow
          label={t("settings.color.bg")}
          value={customTheme.bg}
          onChange={(bg) => setCustomTheme({ bg })}
        />
        <ColorRow
          label={t("settings.color.fg")}
          value={customTheme.fg}
          onChange={(fg) => setCustomTheme({ fg })}
        />
      </div>
    </SettingRow>
  );
}
