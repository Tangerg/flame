import * as stylex from "@stylexjs/stylex";
import type { SegmentedOption } from "@/ui";
import { Checkbox, DropdownMenu, Icon, Segmented, SelectTrigger, vocab } from "@/ui";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@/lib/typography";
import { useT } from "@/lib/i18n";
import { useSystemFonts } from "../application/systemFonts";
import { useFontPreferences } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";

interface FontPickerProps {
  label: string;
  mono: boolean;
  value: string;
  onChange: (v: string) => void;
  defaultLabel: string;
}

const fsx = stylex.create({
  // One measure for the three field labels so the controls beside them start on one line.
  pickerRow: {
    display: "grid",
    gridTemplateColumns: "60px auto 1fr",
    alignItems: "center",
    gap: space.s2,
  },
  sizeRow: {
    display: "grid",
    gridTemplateColumns: "60px 1fr",
    alignItems: "center",
    gap: space.s2,
  },
  legend: { color: color.fgFaint, fontWeight: weight.semibold },
  trigger: { maxWidth: "280px" },
  toEdge: { justifySelf: "end" },
  fields: { display: "grid", gap: space.s2 },
  afterFields: { marginTop: space.s1 },
});

function FontPicker({ label, mono, value, onChange, defaultLabel }: FontPickerProps) {
  const t = useT();
  const fonts = useSystemFonts(mono);
  const customEnabled = value !== "";
  const triggerLabel = customEnabled ? value : defaultLabel;

  return (
    <div {...stylex.props(fsx.pickerRow)}>
      <span {...stylex.props(fsx.legend, typeStep.uiMd)}>{label}</span>
      <Checkbox
        checked={customEnabled}
        onCheckedChange={(c) => onChange(c ? (fonts[0] ?? "") : "")}
        label={t("font.useCustom")}
      />
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={triggerLabel}
              disabled={!customEnabled}
              style={customEnabled ? { fontFamily: `"${value}"` } : undefined}
              className={stylex.props(fsx.trigger, mono && customEnabled && face.mono).className}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          {fonts.map((f) => (
            <DropdownMenu.Item
              key={f}
              onClick={() => onChange(f)}
              style={{ fontFamily: `"${f}"` }}
              layout="pickPlain"
            >
              <span {...stylex.props(vocab.truncate)}>{f}</span>
              {value === f ? (
                <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
              ) : (
                <span aria-hidden />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </div>
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

function FontSizeField({
  label,
  value,
  onChange,
  resetLabel,
}: {
  label: string;
  value: number | null;
  onChange: (v: number | null) => void;
  resetLabel: string;
}) {
  const options: SegmentedOption<string>[] = [
    { value: SIZE_RESET, label: resetLabel },
    ...SIZE_VALUES.map((px) => ({ value: String(px), label: String(px) })),
  ];
  return (
    <div {...stylex.props(fsx.sizeRow)}>
      <span {...stylex.props(fsx.legend, typeStep.uiMd)}>{label}</span>
      <Segmented
        className={stylex.props(fsx.toEdge).className}
        value={value === null ? SIZE_RESET : String(value)}
        options={options}
        onChange={(v) => onChange(v === SIZE_RESET ? null : Number(v))}
        ariaLabel={label}
      />
    </div>
  );
}

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

  return (
    <SettingRow label={t("settings.font")} sub={t("settings.font.sub")} align="start">
      <div {...stylex.props(fsx.fields)}>
        <FontPicker
          label={t("settings.font.ui")}
          mono={false}
          value={uiFont}
          onChange={setUiFont}
          defaultLabel={t("settings.font.defaultUi")}
        />
        <FontPicker
          label={t("settings.font.code")}
          mono={true}
          value={codeFont}
          onChange={setCodeFont}
          defaultLabel={t("settings.font.defaultMono")}
        />
        <FontSizeField
          label={t("settings.font.size")}
          value={fontSize}
          onChange={setFontSize}
          resetLabel={t("settings.font.default")}
        />
        <Checkbox
          checked={fontSmoothing}
          onCheckedChange={setFontSmoothing}
          label={t("settings.font.smoothing")}
          className={stylex.props(fsx.afterFields).className}
        />
      </div>
    </SettingRow>
  );
}
