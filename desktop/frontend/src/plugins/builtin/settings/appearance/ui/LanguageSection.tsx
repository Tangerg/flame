import * as stylex from "@stylexjs/stylex";
import { DropdownMenu, Icon, SelectTrigger } from "@/ui";
import { useLocale, useT } from "@/lib/i18n";
import { LOCALE, useExtensionPoint } from "@/plugins/sdk";
import { selectLocale } from "../application/localeSelection";
import { SettingRow } from "../../kit";
import { settingStyles as ss } from "../../kit/settingStyles";

export function LanguageSection() {
  const t = useT();
  const locale = useLocale();
  const locales = useExtensionPoint(LOCALE);
  const active = locales.find((l) => l.id === locale) ?? locales[0];
  if (!active) return null;

  return (
    <SettingRow label={t("settings.language.label")} sub={t("settings.language.sub")}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={<SelectTrigger label={active.label} aria-label={t("settings.language.label")} />}
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          {locales.map((l) => (
            <DropdownMenu.Item key={l.id} onClick={() => void selectLocale(l)} layout="pickPlain">
              <span {...stylex.props(ss.truncate)}>{l.label}</span>
              {locale === l.id ? (
                <Icon name="check" size="xs" className={stylex.props(ss.accent).className} />
              ) : (
                <span aria-hidden />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </SettingRow>
  );
}
