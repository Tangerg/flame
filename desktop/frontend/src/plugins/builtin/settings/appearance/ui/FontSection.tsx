import * as stylex from "@stylexjs/stylex";
import type { SegmentedOption } from "@/ui";
import { DropdownMenu, Icon, Segmented, SelectTrigger, Switch, vocab } from "@/ui";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@/lib/typography";
import { useT } from "@/lib/i18n";
import { useSystemFonts } from "../application/systemFonts";
import { useFontPreferences } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { face } from "@/styles/tokens.stylex";

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
            className={stylex.props(fsx.trigger, mono && custom && face.mono).className}
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={4}>
        {[SYSTEM, ...fonts].map((family) => (
          <DropdownMenu.Item
            key={family || "system"}
            onClick={() => onChange(family)}
            style={family ? { fontFamily: `"${family}"` } : undefined}
            layout="pickPlain"
          >
            <span {...stylex.props(vocab.truncate)}>{family || defaultLabel}</span>
            {value === family ? (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            ) : (
              <span aria-hidden />
            )}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

const SIZE_VALUES = [
  UI_FONT_SIZE_MIN_PX,
  12,
  13,
  14,
  15,
  16,
  UI_FONT_SIZE_MAX_PX,
] as const satisfies readonly number[];
const SIZE_RESET = "default";

export function FontSection() {
  const t = useT();
  const {
    uiFont,
    codeFont,
    fontSize,
    fontSmoothing,
    setUiFont,
    setCodeFont,
    setFontSize,
    setFontSmoothing,
  } = useFontPreferences();

  const sizeOptions: SegmentedOption<string>[] = [
    { value: SIZE_RESET, label: t("settings.font.default") },
    ...SIZE_VALUES.map((px) => ({ value: String(px), label: String(px) })),
  ];

  return (
    <>
      <SettingRow label={t("settings.font.ui")} sub={t("settings.font.ui.sub")}>
        <FontPicker
          label={t("settings.font.ui")}
          mono={false}
          value={uiFont}
          onChange={setUiFont}
          defaultLabel={t("settings.font.defaultUi")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.code")} sub={t("settings.font.code.sub")}>
        <FontPicker
          label={t("settings.font.code")}
          mono={true}
          value={codeFont}
          onChange={setCodeFont}
          defaultLabel={t("settings.font.defaultMono")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.size")} sub={t("settings.font.size.sub")}>
        <Segmented
          value={fontSize === null ? SIZE_RESET : String(fontSize)}
          options={sizeOptions}
          onChange={(v) => setFontSize(v === SIZE_RESET ? null : Number(v))}
          ariaLabel={t("settings.font.size")}
        />
      </SettingRow>
      <SettingRow label={t("settings.font.smoothing")} sub={t("settings.font.smoothing.sub")}>
        <Switch
          checked={fontSmoothing}
          onCheckedChange={setFontSmoothing}
          ariaLabel={t("settings.font.smoothing")}
        />
      </SettingRow>
    </>
  );
}
