import { UI_DENSITY_MODES } from "@/features/theme/public/appearance";
import { Segmented } from "@flame/ui";
import { useT, type MessageKey } from "@/lib/i18n/index";
import { useAppearance, useAppearanceEdit } from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
const DENSITY_OPTIONS = UI_DENSITY_MODES.map((mode) => ({
  value: mode,
  label: `settings.density.${mode}` as const,
}));

const RADIUS_OPTIONS: { value: number; label: MessageKey }[] = [
  { value: 0.6, label: "shape.opt.sharp" },
  { value: 1, label: "shape.radius.default" },
  { value: 1.4, label: "shape.opt.soft" },
];

const MOTION_OPTIONS: { value: number; label: MessageKey }[] = [
  { value: 0, label: "shape.opt.off" },
  { value: 0.6, label: "shape.opt.fast" },
  { value: 1, label: "shape.motion.default" },
  { value: 1.5, label: "shape.opt.slow" },
];

export function ShapeMotionSection() {
  const t = useT();
  const density = useAppearance((s) => s.density);
  const radiusScale = useAppearance((s) => s.radiusScale);
  const motionScale = useAppearance((s) => s.motionScale);
  const { setDensity, setRadiusScale, setMotionScale } = useAppearanceEdit();

  return (
    <>
      <SettingRow label={t("settings.density")}>
        <Segmented
          value={density}
          options={DENSITY_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setDensity}
          ariaLabel={t("settings.density")}
        />
      </SettingRow>
      <SettingRow label={t("settings.radius")}>
        <Segmented
          value={radiusScale}
          options={RADIUS_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setRadiusScale}
          ariaLabel={t("shape.radius.aria")}
        />
      </SettingRow>
      <SettingRow label={t("settings.motion")}>
        <Segmented
          value={motionScale}
          options={MOTION_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setMotionScale}
          ariaLabel={t("shape.motion.aria")}
        />
      </SettingRow>
    </>
  );
}
