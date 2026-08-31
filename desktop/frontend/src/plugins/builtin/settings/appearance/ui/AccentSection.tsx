import { useId } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { ACCENT, useExtensionPoint } from "@/plugins/sdk";
import { useAccentPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { ColorPickerInput, Icon, Pressable } from "@/ui";

const RAINBOW_HINT =
  "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

const SWATCH_TARGET =
  "group/accent relative inline-grid h-7 w-7 place-items-center rounded-full bg-transparent transition-[background-color,transform] duration-[var(--dur-fast)] hover:bg-hover active:scale-[var(--press-scale)]";
const SWATCH_CHROME =
  "h-5 w-5 rounded-full border-2 border-transparent bg-clip-padding transition-[transform,box-shadow] duration-[var(--dur-fast)] group-hover/accent:scale-105";
const SWATCH_SELECTED = "border-surface shadow-[var(--shadow-swatch-selected)]";

function SelectionMark({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden
      data-slot="accent-selection-mark"
      className={cn(
        "pointer-events-none absolute text-surface transition-[opacity,transform] duration-[var(--dur-fast)]",
        selected ? "scale-100 opacity-100" : "scale-75 opacity-0",
      )}
    >
      <Icon name="check" size="xs" />
    </span>
  );
}

function CustomAccentPicker({
  value,
  isActive,
  onChange,
  label,
}: {
  value: string;
  isActive: boolean;
  onChange: (hex: string) => void;
  label: string;
}) {
  const inputId = useId();
  return (
    <label htmlFor={inputId} title={label} aria-label={label} className={SWATCH_TARGET}>
      <span
        aria-hidden
        className={cn(SWATCH_CHROME, isActive && SWATCH_SELECTED)}
        style={{ background: isActive ? value : RAINBOW_HINT }}
      />
      <SelectionMark selected={isActive} />
      <ColorPickerInput
        id={inputId}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  );
}

export function AccentSection() {
  const t = useT();
  const accents = useExtensionPoint(ACCENT);
  const { accent, setAccent, scheme } = useAccentPreference();
  const light = scheme === "light";

  const isCustom = !accents.some((a) => a.dark === accent);

  return (
    <SettingRow label={t("settings.accent")} sub={t("settings.accent.sub")}>
      <div className="flex flex-wrap gap-2.5 justify-start items-center">
        {accents.map((a) => (
          <Pressable
            key={a.id}
            type="button"
            onClick={() => setAccent(a.dark)}
            title={`${t("settings.accent")}: ${a.label}`}
            aria-label={`${t("settings.accent")}: ${a.label}`}
            aria-pressed={accent === a.dark}
            className={cn("p-0", SWATCH_TARGET)}
          >
            <span
              aria-hidden
              className={cn(SWATCH_CHROME, accent === a.dark && SWATCH_SELECTED)}
              style={{ background: light ? (a.light ?? a.dark) : a.dark }}
            />
            <SelectionMark selected={accent === a.dark} />
          </Pressable>
        ))}
        <CustomAccentPicker
          value={accent}
          isActive={isCustom}
          onChange={setAccent}
          label={t("settings.accent.custom")}
        />
      </div>
    </SettingRow>
  );
}
