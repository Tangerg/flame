import type { SegmentedOption } from "@/ui";
import { Checkbox, DropdownMenu, Icon, Segmented, SelectTrigger } from "@/ui";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@/lib/typography";
import { useT } from "@/lib/i18n";
import { useSystemFonts } from "../application/systemFonts";
import { cn } from "@/lib/classNames";
import { useFontPreferences } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";

interface FontPickerProps {
  label: string;
  mono: boolean;
  value: string;
  onChange: (v: string) => void;
  defaultLabel: string;
}

function FontPicker({ label, mono, value, onChange, defaultLabel }: FontPickerProps) {
  const t = useT();
  const fonts = useSystemFonts(mono);
  const customEnabled = value !== "";
  const triggerLabel = customEnabled ? value : defaultLabel;

  return (
    <div className="grid grid-cols-[60px_auto_1fr] items-center gap-2">
      <span className="text-ui-md font-semibold text-fg-faint">{label}</span>
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
              className={cn(
                "min-w-[var(--select-min-width)] max-w-[280px]",
                mono && customEnabled && "font-mono",
              )}
            />
          }
        />
        <DropdownMenu.Content
          align="start"
          sideOffset={4}
          className="max-h-[280px] min-w-[var(--menu-min-width)] overflow-auto"
        >
          {fonts.map((f) => (
            <DropdownMenu.Item
              key={f}
              onClick={() => onChange(f)}
              style={{ fontFamily: `"${f}"` }}
              className="grid-cols-[minmax(0,1fr)_12px]"
            >
              <span className="truncate">{f}</span>
              {value === f ? (
                <Icon name="check" size="xs" className="text-accent" />
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
    <div className="grid grid-cols-[60px_1fr] items-center gap-2">
      <span className="text-ui-md font-semibold text-fg-faint">{label}</span>
      {/* The pane hangs every control off the card's inner edge — `SettingRow` does it for the
          rows this one nests inside. A content-width control left in its cell stops short of
          that edge and reads as the one row that missed the line. */}
      <Segmented
        className="justify-self-end"
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
      <div className="grid gap-2">
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
          className="mt-1"
        />
      </div>
    </SettingRow>
  );
}
