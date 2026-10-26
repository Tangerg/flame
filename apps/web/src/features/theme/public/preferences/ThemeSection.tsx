import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { type ReactNode } from "react";
import { type Scheme } from "@/lib/appearance";
import { type ColorThemeSpec } from "@/features/theme/public/types";
import { DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SelectTrigger } from "@/ui/index";
import { vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { DEFAULT_ACCENT } from "@/features/theme/public/appearance";
import { solidFill } from "@/features/theme/kit/scale";
import { SettingRow } from "@/ui/settings/SettingRow";
import { useAppearance, useAppearanceEdit } from "@/features/theme/public/appearance";
import { color, corner, radius, space, type as typeStep } from "@flame/ui/tokens.stylex";
const FALLBACK_TOKENS: Record<Scheme, { bg: string; surface: string }> = {
  dark: { bg: "#0c0d0f", surface: "#16181b" },
  light: { bg: "#ffffff", surface: "#f6f7f8" },
};

function previewTokens(spec: ColorThemeSpec): { bg: string; surface: string; accent: string } {
  const fallback = FALLBACK_TOKENS[spec.scheme];
  return {
    bg: spec.tokens?.["color-bg"] ?? fallback.bg,
    surface: spec.tokens?.["color-surface"] ?? fallback.surface,
    accent: spec.tokens?.["color-accent"] ?? solidFill(DEFAULT_ACCENT),
  };
}

const th = stylex.create({
  swatch: {
    position: "relative",
    display: "block",
    height: space.s4,
    width: space.s6,
    flexShrink: 0,
    overflow: "hidden",
    borderRadius: radius.step2xs,
  },
  swatchPane: {
    position: "absolute",
    insetInline: "3px",
    top: "3px",
    bottom: "2px",
    borderRadius: "max(0px, calc(var(--shape-2xs) - 3px))",
  },
  swatchDot: {
    position: "absolute",
    bottom: "2px",
    right: "2px",
    height: space.s1,
    width: space.s1,
  },
  half: { position: "absolute", insetBlock: 0, width: "50%" },
  halfStart: { left: 0 },
  halfEnd: { right: 0 },
  name: { color: color.fg },
});

function ThemeSwatch({ bg, surface, accent }: { bg: string; surface: string; accent: string }) {
  return (
    <span
      className={cn("media-edge", stylex.props(th.swatch).className)}
      style={{ background: bg }}
    >
      <span {...stylex.props(th.swatchPane)} style={{ background: surface }} />
      <span {...stylex.props(th.swatchDot, corner.pill)} style={{ background: accent }} />
    </span>
  );
}

function SystemSwatch() {
  return (
    <span {...stylex.props(th.swatch)}>
      <span
        {...stylex.props(th.half, th.halfStart)}
        style={{ background: FALLBACK_TOKENS.dark.bg }}
      />
      <span
        {...stylex.props(th.half, th.halfEnd)}
        style={{ background: FALLBACK_TOKENS.light.bg }}
      />
    </span>
  );
}

function ThemeItem({ swatch, label, value }: { swatch: ReactNode; label: string; value: string }) {
  return (
    <DropdownMenu.RadioItem value={value} label={label} layout="pickWide">
      {swatch}
      <span {...stylex.props(th.name, vocab.truncate, typeStep.uiMd)}>{label}</span>
      <DropdownMenu.RadioItemIndicator>
        <Icon name="check" size="sm" className={stylex.props(vocab.accent).className} />
      </DropdownMenu.RadioItemIndicator>
    </DropdownMenu.RadioItem>
  );
}

export function ThemeSection() {
  const t = useT();
  const themes = useExtensionPoint(COLOR_THEME);
  const theme = useAppearance((s) => s.theme);
  const { setTheme } = useAppearanceEdit();

  const isSystem = theme === "system";
  const activeSpec = themes.find((s) => s.id === theme);
  const triggerLabel = isSystem || !activeSpec ? t("settings.theme.system") : activeSpec.label;
  const triggerSwatch =
    isSystem || !activeSpec ? <SystemSwatch /> : <ThemeSwatch {...previewTokens(activeSpec)} />;

  return (
    <SettingRow label={t("settings.theme")}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={triggerLabel}
              leading={triggerSwatch}
              aria-label={t("settings.theme")}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          <DropdownMenu.RadioGroup value={theme} onValueChange={(id) => setTheme(id)}>
            <ThemeItem
              swatch={<SystemSwatch />}
              label={t("settings.theme.system")}
              value="system"
            />
            {themes.map((spec) => (
              <ThemeItem
                key={spec.id}
                swatch={<ThemeSwatch {...previewTokens(spec)} />}
                label={spec.label}
                value={spec.id}
              />
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </SettingRow>
  );
}
