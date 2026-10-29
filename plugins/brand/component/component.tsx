import { useMemo, useRef, useState } from "react";
import { companion, FLAME_ORANGE, mark, plush } from "@flame/brand-assets";
import {
  createTranslator,
  failureText,
  type ComponentContext,
} from "@flame/component-sdk";
import { Check, Copy, Download } from "@flame/icons/react";
import {
  Button,
  DocumentBody,
  Facts,
  Icon,
  PanelSection,
  RecordBody,
  RecordGroup,
  Tile,
  TileGrid,
  useCopyFeedback,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
const assets = [
  { id: "mark", source: mark },
  { id: "companion", source: companion },
  { id: "plush", source: plush },
];
const hex = FLAME_ORANGE.toUpperCase();
const rgb = [1, 3, 5]
  .map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16))
  .join(", ");

function pngBytes(source: string): Uint8Array {
  const prefix = "data:image/png;base64,";
  if (!source.startsWith(prefix))
    throw new Error("brand: PNG was not embedded by the component build");
  return Uint8Array.from(atob(source.slice(prefix.length)), (value) =>
    value.charCodeAt(0),
  );
}

export function Brand({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const { copied, copy } = useCopyFeedback(hex);
  const saving = useRef(false);
  const [savingID, setSavingID] = useState<string>();
  const [failure, setFailure] = useState<string>();

  async function download(asset: (typeof assets)[number]) {
    if (saving.current || context.signal.aborted) return;
    saving.current = true;
    setSavingID(asset.id);
    setFailure(undefined);
    try {
      await context.saveFile({
        filename: `flame-${asset.id}.png`,
        mime: "image/png",
        bytes: pngBytes(asset.source),
      });
    } catch (error) {
      if (!context.signal.aborted)
        setFailure(`${t("downloadFailed")} ${failureText(error)}`);
    } finally {
      saving.current = false;
      if (!context.signal.aborted) setSavingID(undefined);
    }
  }

  return (
    <>
      {failure && <p role="alert">{failure}</p>}
      <PanelSection title={t("assets")} description={t("description")}>
        <TileGrid aria-label={t("title")}>
          {assets.map((asset) => (
            <Tile
              key={asset.id}
              visual={
                <img
                  src={asset.source}
                  alt={t(`${asset.id}.title`)}
                  width={1024}
                  height={1024}
                  style={{
                    display: "block",
                    width: "100%",
                    height: "auto",
                    aspectRatio: "1",
                    objectFit: "contain",
                  }}
                />
              }
            >
              <strong>{t(`${asset.id}.title`)}</strong>
              <RecordBody>{t(`${asset.id}.description`)}</RecordBody>
              <Button
                variant="soft"
                size="sm"
                pending={savingID === asset.id}
                disabled={savingID !== undefined && savingID !== asset.id}
                aria-label={t("download", { name: t(`${asset.id}.title`) })}
                onClick={() => void download(asset)}
              >
                <Icon glyph={Download} />
                {t("downloadPng")}
              </Button>
            </Tile>
          ))}
        </TileGrid>
      </PanelSection>
      <PanelSection
        title={t("color.title")}
        description={t("color.description")}
      >
        <RecordGroup
          variant="list"
          style={{ padding: "var(--layout-card-padding)" }}
        >
          <div
            style={{
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              gap: "var(--layout-content-gap)",
            }}
          >
            <span
              aria-hidden="true"
              style={{
                width: "6rem",
                height: "6rem",
                flexShrink: 0,
                borderRadius: "var(--radius-lg)",
                backgroundColor: FLAME_ORANGE,
              }}
            />
            <div style={{ flex: "1 1 12rem", minWidth: 0 }}>
              <strong>{t("color.name")}</strong>
              <Facts
                values={[
                  [
                    "HEX",
                    <code key="hex" style={{ userSelect: "all" }}>
                      {hex}
                    </code>,
                  ],
                  [
                    "RGB",
                    <code key="rgb" style={{ userSelect: "all" }}>
                      {rgb}
                    </code>,
                  ],
                ]}
              />
            </div>
            <Button
              variant="soft"
              size="sm"
              onClick={() => {
                if (context.signal.aborted) return;
                setFailure(undefined);
                void copy().then((accepted) => {
                  if (!accepted && !context.signal.aborted)
                    setFailure(t("copyFailed"));
                });
              }}
            >
              <Icon glyph={copied ? Check : Copy} />
              <span aria-live="polite">{t(copied ? "copied" : "copyHex")}</span>
            </Button>
          </div>
        </RecordGroup>
      </PanelSection>
      <PanelSection title={t("guidelines.title")}>
        <DocumentBody>
          <ul>
            <li>{t("guidelines.shape")}</li>
            <li>{t("guidelines.color")}</li>
          </ul>
        </DocumentBody>
      </PanelSection>
    </>
  );
}
