import * as stylex from "@stylexjs/stylex";
import { colord } from "colord";
import { useState } from "react";
import { Button, CARD_SURFACE, PanelSection, useCopyFeedback } from "@flame/ui";
import { color, face, layout, radius, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { brandAssets } from "@/lib/brand";
import { FLAME_ORANGE } from "@/lib/brand";
import { brandColor } from "@/lib/brand.stylex";
import { useT } from "@/lib/i18n/index";
import { notifyError } from "@/features/notifications/public/notifications";
import { Icon } from "@/ui/index";
const styles = stylex.create({
  grid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 12rem), 1fr))",
    gap: layout.contentGap,
    minWidth: 0,
  },
  card: { margin: 0, minWidth: 0, overflow: "hidden" },
  preview: {
    display: "grid",
    placeItems: "center",
    aspectRatio: "1",
    padding: space.s6,
    backgroundColor: surface.sunken,
  },
  image: { display: "block", width: "100%", height: "100%", objectFit: "contain" },
  caption: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    padding: layout.cardPadding,
  },
  title: { margin: 0, color: color.fg, fontWeight: weight.semibold },
  description: { margin: 0, color: color.fgMuted, textWrap: "pretty" },
  download: { marginTop: space.s2 },
  colorCard: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.contentGap,
    padding: layout.cardPadding,
  },
  swatch: {
    width: space.s24,
    height: space.s24,
    flexShrink: 0,
    borderRadius: radius.lg,
    backgroundColor: brandColor.orange,
  },
  colorDetails: { display: "grid", gap: space.s2, flex: "1 1 12rem", minWidth: 0 },
  values: {
    display: "flex",
    flexWrap: "wrap",
    gap: space.s4,
    margin: 0,
    color: color.fgSoft,
  },
  value: { display: "grid", gap: space.s1 },
  code: { margin: 0, userSelect: "all", color: color.fg },
  guidelines: {
    display: "grid",
    gap: space.s2,
    margin: 0,
    paddingInlineStart: space.s5,
    color: color.fgMuted,
    textWrap: "pretty",
  },
});

const assets = [
  {
    id: "mark",
    source: brandAssets.mark,
    name: "settings.brand.mark.title",
    description: "settings.brand.mark.description",
  },
  {
    id: "companion",
    source: brandAssets.companion,
    name: "settings.brand.companion.title",
    description: "settings.brand.companion.description",
  },
  {
    id: "plush",
    source: brandAssets.plush,
    name: "settings.brand.plush.title",
    description: "settings.brand.plush.description",
  },
];

const hex = FLAME_ORANGE.toUpperCase();

const { r, g, b } = colord(FLAME_ORANGE).toRgb();

const rgb = `${r}, ${g}, ${b}`;

export function BrandPane({ saveImage }: { saveImage: (source: string) => Promise<boolean> }) {
  const t = useT();
  const { copied, copy } = useCopyFeedback(hex);
  const [saving, setSaving] = useState(false);

  async function copyColor() {
    if (!(await copy())) notifyError(t("settings.brand.copyFailed"));
  }

  async function downloadAsset(source: string) {
    if (saving) return;
    setSaving(true);
    try {
      const response = await fetch(source);
      if (!response.ok) throw new Error(`brand image request failed: ${response.status}`);
      const blob = await response.blob();
      const image = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
          if (typeof reader.result === "string") resolve(reader.result);
          else reject(new Error("brand image read returned no data"));
        };
        reader.onerror = () => reject(reader.error ?? new Error("brand image read failed"));
        reader.readAsDataURL(blob);
      });
      await saveImage(image);
    } catch {
      notifyError(t("settings.brand.downloadFailed"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <PanelSection title={t("settings.brand.assets")}>
        <div {...stylex.props(styles.grid)}>
          {assets.map((asset) => (
            <figure key={asset.id} {...stylex.props(CARD_SURFACE.face, styles.card)}>
              <div {...stylex.props(styles.preview)}>
                <img
                  src={asset.source}
                  alt={t(asset.name)}
                  width={1024}
                  height={1024}
                  {...stylex.props(styles.image)}
                />
              </div>
              <figcaption {...stylex.props(styles.caption)}>
                <h3 {...stylex.props(styles.title, type.uiMd)}>{t(asset.name)}</h3>
                <p {...stylex.props(styles.description, type.uiSm)}>{t(asset.description)}</p>
                <Button
                  variant="soft"
                  size="sm"
                  pending={saving}
                  onClick={() => void downloadAsset(asset.source)}
                  aria-label={t("settings.brand.download", { name: t(asset.name) })}
                  styles={styles.download}
                >
                  <Icon name="download" />
                  {t("settings.brand.downloadPng")}
                </Button>
              </figcaption>
            </figure>
          ))}
        </div>
      </PanelSection>
      <PanelSection
        title={t("settings.brand.color.title")}
        description={t("settings.brand.color.description")}
      >
        <div {...stylex.props(CARD_SURFACE.face, styles.colorCard)}>
          <div aria-hidden="true" {...stylex.props(styles.swatch)} />
          <div {...stylex.props(styles.colorDetails)}>
            <h3 {...stylex.props(styles.title, type.uiMd)}>{t("settings.brand.color.name")}</h3>
            <dl {...stylex.props(styles.values, type.uiSm)}>
              <div {...stylex.props(styles.value)}>
                <dt>HEX</dt>
                <dd {...stylex.props(styles.code, face.mono)}>{hex}</dd>
              </div>
              <div {...stylex.props(styles.value)}>
                <dt>RGB</dt>
                <dd {...stylex.props(styles.code, face.mono)}>{rgb}</dd>
              </div>
            </dl>
          </div>
          <Button variant="soft" size="sm" onClick={() => void copyColor()}>
            <Icon name={copied ? "check" : "copy"} />
            <span aria-live="polite">
              {t(copied ? "settings.brand.copied" : "settings.brand.copyHex")}
            </span>
          </Button>
        </div>
      </PanelSection>
      <PanelSection title={t("settings.brand.guidelines.title")}>
        <ul {...stylex.props(styles.guidelines, type.uiSm)}>
          <li>{t("settings.brand.guidelines.shape")}</li>
          <li>{t("settings.brand.guidelines.color")}</li>
        </ul>
      </PanelSection>
    </>
  );
}
