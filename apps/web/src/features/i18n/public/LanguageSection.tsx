import * as stylex from "@stylexjs/stylex";
import { useRef } from "react";
import { DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SelectTrigger } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { vocab } from "@flame/ui";
import { useLocale, useT } from "@/lib/i18n/index";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { type LocaleSpec } from "@/features/i18n/public/types";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { selectLocale } from "@/features/i18n/application/localeSelection";
import { SettingRow } from "@/ui/settings/SettingRow";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
export function LanguageSection() {
  const t = useT();
  const locale = useLocale();
  const locales = useExtensionPoint(LOCALE);
  const { feedback, run } = useAsyncFeedback(locales);
  const trigger = useRef<HTMLButtonElement>(null);
  const active = locales.find((l) => l.id === locale);
  if (locales.length === 0) return null;

  const choose = (chosen: LocaleSpec) => {
    void run(async () => {
      await selectLocale(chosen);
      return { ok: true };
    }, t("settings.language.label"));
  };

  return (
    <SettingRow label={t("settings.language.label")}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              ref={trigger}
              label={active?.label ?? locale}
              aria-label={t("settings.language.label")}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          <DropdownMenu.RadioGroup
            value={locale}
            onValueChange={(id) => {
              const chosen = locales.find((l) => l.id === id);
              if (chosen) choose(chosen);
            }}
          >
            {locales.map((l) => (
              <DropdownMenu.RadioItem key={l.id} value={l.id} label={l.label} layout="pickPlain">
                <span {...stylex.props(vocab.truncate)}>{l.label}</span>
                <DropdownMenu.RadioItemIndicator>
                  <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
                </DropdownMenu.RadioItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      {feedback.state === "busy" && <SystemMessage hideIcon>{t("common.loading")}</SystemMessage>}
      {feedback.state === "error" && (
        <SystemMessage
          variant="warning"
          action={
            active
              ? {
                  label: t("common.retry"),
                  onClick: () => {
                    trigger.current?.focus();
                    choose(active);
                  },
                }
              : undefined
          }
        >
          {feedback.reason}
        </SystemMessage>
      )}
    </SettingRow>
  );
}
