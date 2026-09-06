import type { ReactNode } from "react";
import type { Scheme } from "@/lib/appearance";
import type { ColorThemeSpec } from "@/plugins/sdk";
import { DropdownMenu, Icon, SelectTrigger } from "@/ui";
import { useT } from "@/lib/i18n";
import { COLOR_THEME, useExtensionPoint } from "@/plugins/sdk";
import { SettingRow } from "../../kit";
import { useThemePreference } from "../application/appearancePreferences";

const FALLBACK_TOKENS: Record<Scheme, { bg: string; surface: string; accent: string }> = {
  dark: { bg: "#0c0d0f", surface: "#16181b", accent: "#6c97ff" },
  light: { bg: "#ffffff", surface: "#f6f7f8", accent: "#2563eb" },
};

function previewTokens(spec: ColorThemeSpec): { bg: string; surface: string; accent: string } {
  const fallback = FALLBACK_TOKENS[spec.scheme];
  return {
    bg: spec.tokens?.["color-bg"] ?? fallback.bg,
    surface: spec.tokens?.["color-surface"] ?? fallback.surface,
    accent: spec.tokens?.["color-accent"] ?? fallback.accent,
  };
}

function ThemeSwatch({ bg, surface, accent }: { bg: string; surface: string; accent: string }) {
  return (
    <span
      className="relative block h-4 w-6 shrink-0 overflow-hidden rounded-2xs media-edge"
      style={{ background: bg }}
    >
      <span
        className="absolute inset-x-[3px] top-[3px] bottom-[2px] rounded-[max(0px,calc(var(--shape-2xs)-3px))]"
        style={{ background: surface }}
      />
      <span
        className="absolute bottom-[2px] right-[2px] h-1 w-1 rounded-full"
        style={{ background: accent }}
      />
    </span>
  );
}

function SystemSwatch() {
  return (
    <span className="relative block h-4 w-6 shrink-0 overflow-hidden rounded-2xs media-edge">
      <span
        className="absolute inset-y-0 left-0 w-1/2"
        style={{ background: FALLBACK_TOKENS.dark.bg }}
      />
      <span
        className="absolute inset-y-0 right-0 w-1/2"
        style={{ background: FALLBACK_TOKENS.light.bg }}
      />
    </span>
  );
}

function ThemeItem({
  swatch,
  label,
  active,
  onSelect,
}: {
  swatch: ReactNode;
  label: string;
  active: boolean;
  onSelect: () => void;
}) {
  return (
    <DropdownMenu.Item className="grid-cols-[24px_minmax(0,1fr)_14px]" onClick={onSelect}>
      {swatch}
      <span className="truncate text-ui-md text-fg">{label}</span>
      {active ? <Icon name="check" size="sm" className="text-accent" /> : <span aria-hidden />}
    </DropdownMenu.Item>
  );
}

export function ThemeSection() {
  const t = useT();
  const themes = useExtensionPoint(COLOR_THEME);
  const { theme, setTheme } = useThemePreference();

  const isSystem = theme === "system";
  const activeSpec = themes.find((s) => s.id === theme);
  const triggerLabel = isSystem || !activeSpec ? t("settings.theme.system") : activeSpec.label;
  const triggerSwatch =
    isSystem || !activeSpec ? <SystemSwatch /> : <ThemeSwatch {...previewTokens(activeSpec)} />;

  return (
    <SettingRow label={t("settings.theme")} sub={t("settings.theme.sub")}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={triggerLabel}
              leading={triggerSwatch}
              aria-label={t("settings.theme")}
              className="min-w-[var(--select-min-width)]"
            />
          }
        />
        <DropdownMenu.Content
          align="start"
          sideOffset={4}
          className="max-h-[min(60vh,380px)] min-w-[var(--menu-min-width)] overflow-y-auto"
        >
          <ThemeItem
            swatch={<SystemSwatch />}
            label={t("settings.theme.system")}
            active={isSystem}
            onSelect={() => setTheme("system")}
          />
          {themes.map((spec) => (
            <ThemeItem
              key={spec.id}
              swatch={<ThemeSwatch {...previewTokens(spec)} />}
              label={spec.label}
              active={theme === spec.id}
              onSelect={() => setTheme(spec.id)}
            />
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </SettingRow>
  );
}
