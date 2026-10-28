import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { localeToBcp47, useLocale, useT } from "@/lib/i18n/index";
import {
  DEFAULT_CUSTOM_THEME,
  useAppearance,
  useAppearanceEdit,
} from "@/features/theme/public/appearance";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import { customPalette } from "@/features/theme/kit/customPalette";
import { paletteThemePreview } from "@/features/theme/kit/palette";
import { WCAG_AA_TEXT } from "@/features/theme/kit/legibility";
import { SettingRow } from "@/ui/settings/SettingRow";
import { ColorPickerInput } from "@/ui/index";
import { Button, vocab } from "@flame/ui";
import { color, corner, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const a = stylex.create({
  row: { position: "relative" },
  swatchLine: { display: "inline-flex", alignItems: "center", gap: space.s2 },
  hex: { fontFamily: "var(--font-mono)", textTransform: "uppercase", color: color.fg },
  chip: {
    height: space.s4_5,
    width: space.s4_5,
    borderWidth: "var(--hairline-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundClip: "padding-box",
  },
  colorGrid: { display: "grid", width: "20rem", maxWidth: "100%", gap: space.s3 },
  preview: {
    margin: 0,
    padding: space.s4,
    borderRadius: radius.card,
    overflowWrap: "anywhere",
  },
  feedback: { display: "grid", gap: space.s1 },
  invalid: { color: color.negative },
});

function ColorRow({
  label,
  value,
  onChange,
  description,
}: {
  label: string;
  value: string;
  onChange: (hex: string) => void;
  description: string;
}) {
  return (
    <div data-focus-proxy="" {...stylex.props(ss.sunkenRow, a.row)}>
      <span {...stylex.props(vocab.muted, typeStep.uiMd)}>{label}</span>
      <span {...stylex.props(a.swatchLine)}>
        <span {...stylex.props(a.hex, typeStep.uiMd)}>{value}</span>
        <span
          className={stylex.props(a.chip, corner.pill).className}
          style={{ background: value }}
        />
        <ColorPickerInput
          aria-label={label}
          aria-describedby={description}
          value={value}
          onValueChange={onChange}
        />
      </span>
    </div>
  );
}

export function CustomThemeColors() {
  const theme = useAppearance((s) => s.theme);
  return theme === "custom" ? <ColorEditor /> : null;
}

function ColorEditor() {
  const t = useT();
  const locale = useLocale();
  const labelId = useId();
  const feedbackId = useId();
  const preference = useAppearance((s) => s);
  const { customTheme, accent, contrast } = preference;
  const { setCustomTheme } = useAppearanceEdit();
  const [draft, setDraft] = useState<AppearancePreference["customTheme"]>();
  const colors = draft ?? customTheme;
  const changed =
    colors.bg.toLowerCase() !== customTheme.bg.toLowerCase() ||
    colors.fg.toLowerCase() !== customTheme.fg.toLowerCase();
  const preview = paletteThemePreview(customPalette(colors), accent, contrast, preference);
  const readable = preview.textContrast >= WCAG_AA_TEXT;
  const appliedContrast = changed
    ? paletteThemePreview(customPalette(customTheme), accent, contrast, preference).textContrast
    : preview.textContrast;
  const ratio = new Intl.NumberFormat(localeToBcp47(locale), {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(Math.floor(preview.textContrast * 100) / 100);
  const minimum = new Intl.NumberFormat(localeToBcp47(locale)).format(WCAG_AA_TEXT);

  return (
    <SettingRow label={t("settings.customColors")} labelId={labelId} align="start">
      <div role="group" aria-labelledby={labelId} {...stylex.props(a.colorGrid)}>
        <ColorRow
          label={t("settings.color.bg")}
          value={colors.bg}
          onChange={(bg) => setDraft({ ...colors, bg })}
          description={feedbackId}
        />
        <ColorRow
          label={t("settings.color.fg")}
          value={colors.fg}
          onChange={(fg) => setDraft({ ...colors, fg })}
          description={feedbackId}
        />
        <figure
          aria-label={t("settings.color.preview")}
          {...stylex.props(a.preview, typeStep.uiMd)}
          style={{ backgroundColor: colors.bg, color: colors.fg }}
        >
          {t("settings.color.sample")}
        </figure>
        <div id={feedbackId} role="status" {...stylex.props(a.feedback, typeStep.uiSm)}>
          <span {...stylex.props(vocab.muted)}>{t("settings.color.contrast", { ratio })}</span>
          {!readable && (
            <span {...stylex.props(a.invalid)}>
              {t("settings.color.minimum", { ratio: minimum })}
            </span>
          )}
        </div>
        <div {...stylex.props(ss.lineWrap)}>
          <Button
            variant="primary"
            disabled={!changed || !readable}
            focusableWhenDisabled
            aria-describedby={feedbackId}
            onClick={() => {
              setCustomTheme(colors);
              setDraft(undefined);
            }}
          >
            {t("settings.color.apply")}
          </Button>
          <Button disabled={!changed} focusableWhenDisabled onClick={() => setDraft(undefined)}>
            {t("settings.color.discard")}
          </Button>
          <Button
            variant={appliedContrast < WCAG_AA_TEXT ? "primary" : "ghost"}
            onClick={() => {
              setCustomTheme(DEFAULT_CUSTOM_THEME);
              setDraft(undefined);
            }}
          >
            {t("settings.color.restore")}
          </Button>
        </div>
      </div>
    </SettingRow>
  );
}
