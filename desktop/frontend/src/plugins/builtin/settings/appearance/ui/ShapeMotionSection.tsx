import { UI_DENSITY_MODES, type UiDensity } from "@/lib/density";
import { Segmented, type SegmentedOption } from "@/ui";
import { useT } from "@/lib/i18n";
import { useShapeMotionPreferences } from "../application/appearancePreferences";
import { SettingRow } from "../../public";

const DENSITY_OPTIONS: SegmentedOption<UiDensity>[] = UI_DENSITY_MODES.map((mode) => ({
  value: mode,
  label: `settings.density.${mode}`,
}));

const RADIUS_OPTIONS: SegmentedOption<number>[] = [
  { value: 0.6, label: "shape.opt.sharp" },
  { value: 1, label: "settings.font.default" },
  { value: 1.4, label: "shape.opt.soft" },
];

const MOTION_OPTIONS: SegmentedOption<number>[] = [
  { value: 0, label: "shape.opt.off" },
  { value: 0.6, label: "shape.opt.fast" },
  { value: 1, label: "settings.font.default" },
  { value: 1.5, label: "shape.opt.slow" },
];

export function ShapeMotionSection() {
  const t = useT();
  const { density, setDensity, radiusScale, motionScale, setRadiusScale, setMotionScale } =
    useShapeMotionPreferences();

  return (
    <>
      <SettingRow label={t("settings.density")} sub={t("settings.density.sub")} align="start">
        <Segmented
          value={density}
          options={DENSITY_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setDensity}
          ariaLabel={t("settings.density")}
        />
      </SettingRow>
      <SettingRow label={t("settings.radius")} sub={t("settings.radius.sub")} align="start">
        <Segmented
          value={radiusScale}
          options={RADIUS_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setRadiusScale}
          ariaLabel={t("shape.radius.aria")}
        />
      </SettingRow>
      <SettingRow label={t("settings.motion")} sub={t("settings.motion.sub")} align="start">
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
