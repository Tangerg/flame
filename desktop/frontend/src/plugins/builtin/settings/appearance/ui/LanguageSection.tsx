import * as stylex from "@stylexjs/stylex";
import { DropdownMenu, Icon, SelectTrigger, SystemMessage, vocab } from "@/ui";
import { useLocale, useT } from "@/lib/i18n";
import { LOCALE, useExtensionPoint } from "@/plugins/sdk";
import { selectLocale } from "../application/localeSelection";
import { SettingRow, useAsyncFeedback } from "../../kit";

export function LanguageSection() {
  const t = useT();
  const locale = useLocale();
  const locales = useExtensionPoint(LOCALE);
  const { feedback, run } = useAsyncFeedback(locales);
  const active = locales.find((l) => l.id === locale);
  if (locales.length === 0) return null;

  return (
    <SettingRow label={t("settings.language.label")} sub={t("settings.language.sub")}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={active?.label ?? locale}
              aria-label={t("settings.language.label")}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          {locales.map((l) => (
            <DropdownMenu.Item
              key={l.id}
              onClick={() =>
                void run(async () => {
                  await selectLocale(l);
                  return { ok: true };
                }, t("settings.language.label"))
              }
              layout="pickPlain"
            >
              <span {...stylex.props(vocab.truncate)}>{l.label}</span>
              {locale === l.id ? (
                <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
              ) : (
                <span aria-hidden />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      {feedback.state === "error" && (
        <SystemMessage variant="warning">{feedback.reason}</SystemMessage>
      )}
    </SettingRow>
  );
}
