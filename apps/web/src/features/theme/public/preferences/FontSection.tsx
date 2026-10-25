import * as stylex from "@stylexjs/stylex";
import { DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SelectTrigger } from "@/ui/index";
import { Switch } from "@flame/ui";
import { Select, vocab } from "@flame/ui";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@flame/ui/typography";
import { useT } from "@/lib/i18n/index";
import { useSystemFonts } from "@/features/theme/application/preferences/systemFonts";
import { useAppearance, editAppearance } from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
import { face } from "@flame/ui/tokens.stylex";
const fsx = stylex.create({
  trigger: { maxWidth: "280px" },
});

const SYSTEM = "";

function FontPicker({
  label,
  mono,
  value,
  onChange,
  defaultLabel,
}: {
  label: string;
  mono: boolean;
  value: string;
  onChange: (v: string) => void;
  defaultLabel: string;
}) {
  const fonts = useSystemFonts(mono);
  const custom = value !== SYSTEM;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <SelectTrigger
            label={custom ? value : defaultLabel}
            aria-label={label}
            style={custom ? { fontFamily: `"${value}"` } : undefined}
            styles={[fsx.trigger, mono && custom && face.mono]}
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={4}>
        <DropdownMenu.RadioGroup value={value} onValueChange={onChange}>
          {[SYSTEM, ...fonts].map((family) => (
            <DropdownMenu.RadioItem
              key={family || "system"}
              value={family}
              label={family || defaultLabel}
              style={family ? { fontFamily: `"${family}"` } : undefined}
              layout="pickPlain"
            >
              <span {...stylex.props(vocab.truncate)}>{family || defaultLabel}</span>
              <DropdownMenu.RadioItemIndicator>
                <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
              </DropdownMenu.RadioItemIndicator>
            </DropdownMenu.RadioItem>
          ))}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

const CODE_SIZE_VALUES = [11, 12, 13, 14, 15, 16];

const SIZE_VALUES = Array.from(
  { length: UI_FONT_SIZE_MAX_PX - UI_FONT_SIZE_MIN_PX + 1 },
  (_, index) => UI_FONT_SIZE_MIN_PX + index,
);

const SIZE_RESET = "default";

export function FontSection() {
  const t = useT();
  const uiFont = useAppearance((s) => s.uiFont);
  const codeFont = useAppearance((s) => s.codeFont);
  const fontSize = useAppearance((s) => s.fontSize);
  const codeFontSize = useAppearance((s) => s.codeFontSize);
  const fontSmoothing = useAppearance((s) => s.fontSmoothing);
  const { setUiFont, setCodeFont, setFontSize, setCodeFontSize, setFontSmoothing } =
    editAppearance();

  const codeSizeOptions = [
    { value: SIZE_RESET, label: t("settings.font.followUi") },
    ...CODE_SIZE_VALUES.map((px) => ({ value: String(px), label: String(px) })),
  ];
  const sizeOptions = [
    { value: SIZE_RESET, label: t("settings.font.default") },
    ...SIZE_VALUES.map((px) => ({ value: String(px), label: String(px) })),
  ];

  return (
    <>
      <SettingRow label={t("settings.font.ui")}>
        <FontPicker
          label={t("settings.font.ui")}
          mono={false}
          value={uiFont}
          onChange={setUiFont}
          defaultLabel={t("settings.font.defaultUi")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.code")}>
        <FontPicker
          label={t("settings.font.code")}
          mono={true}
          value={codeFont}
          onChange={setCodeFont}
          defaultLabel={t("settings.font.defaultMono")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.size")}>
        <Select
          value={fontSize === null ? SIZE_RESET : String(fontSize)}
          options={sizeOptions}
          onValueChange={(v) => setFontSize(v === SIZE_RESET ? null : Number(v))}
          label={t("settings.font.size")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.codeSize")}>
        <Select
          value={codeFontSize === null ? SIZE_RESET : String(codeFontSize)}
          options={codeSizeOptions}
          onValueChange={(v) => setCodeFontSize(v === SIZE_RESET ? null : Number(v))}
          label={t("settings.font.codeSize")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.smoothing")}>
        <Switch
          checked={fontSmoothing}
          onCheckedChange={setFontSmoothing}
          ariaLabel={t("settings.font.smoothing")}
        />
      </SettingRow>
    </>
  );
}
