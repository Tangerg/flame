import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEventHandler,
  type ReactElement,
} from "react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import { IconButton, LightboxDialog } from "@/ui";
import { saveInlineImage } from "../adapters/desktopImageSave";
import { MESSAGE_CONTENT_SELECTOR } from "./messageContent";
import { color, corner, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";

const ig = stylex.create({
  frame: {
    position: "relative",
    display: "flex",
    height: "100%",
    width: "100%",
    flexDirection: "column",
  },
  // The controls float over the image on the scrim's own layer, so the image below can be
  // panned and zoomed without them moving with it.
  topRight: {
    position: "absolute",
    top: space.s3,
    right: space.s3,
    zIndex: 1,
    display: "flex",
    alignItems: "center",
    gap: space.s1,
  },
  prev: { position: "absolute", top: "50%", left: space.s3, zIndex: 1, translate: "0 -50%" },
  next: { position: "absolute", top: "50%", right: space.s3, zIndex: 1, translate: "0 -50%" },
  // The pan surface leaves room at top and bottom for the two control clusters.
  pan: {
    minHeight: 0,
    flex: 1,
    overflow: "auto",
    padding: space.s4,
    paddingTop: space.s12,
    paddingBottom: space.s16,
  },
  centre: { display: "grid", minHeight: "100%", minWidth: "100%", placeItems: "center" },
  image: {
    display: "block",
    maxHeight: "calc(100dvh - 8rem)",
    maxWidth: "calc(100vw - 2rem)",
    borderRadius: radius.lg,
    objectFit: "contain",
  },
  tray: {
    boxShadow: "var(--shadow-floating)",
    position: "absolute",
    bottom: space.s3,
    left: "50%",
    zIndex: 1,
    display: "flex",
    translate: "-50% 0",
    alignItems: "center",
    gap: space.s1,
    backgroundColor: surface.mediaScrim,
    padding: space.s1,
    color: color.onMedia,
  },
  // A measure the percentage cannot outgrow, so the buttons beside it hold still.
  zoom: {
    minWidth: "calc(var(--spacing) * 14)",
    paddingInline: space.s1,
    textAlign: "center",
    fontFamily: "var(--font-mono)",
  },
});

const PREVIEW_TRIGGER_ATTR = "data-message-image-preview-trigger";
const ZOOM_STEPS = [100, 125, 150, 200, 300, 400] as const;

interface GalleryItem {
  src: string;
  alt: string;
  title?: string;
  width?: number;
  height?: number;
}

interface GalleryState {
  items: GalleryItem[];
  index: number;
}

interface FittedImageSize {
  src: string;
  width: number;
  height: number;
}

interface TriggerProps {
  "data-message-image-preview-trigger": "true";
  onClick: MouseEventHandler<HTMLButtonElement>;
}

interface Props {
  item: GalleryItem;
  titleFallback: string;
  trigger: (props: TriggerProps) => ReactElement;
}

function numericAttribute(image: HTMLImageElement, name: "width" | "height") {
  const value = image.getAttribute(name);
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function galleryRoot(trigger: HTMLButtonElement) {
  return trigger.closest(MESSAGE_CONTENT_SELECTOR) ?? trigger.closest(".md");
}

function collectGallery(trigger: HTMLButtonElement, fallback: GalleryItem): GalleryState {
  const root = galleryRoot(trigger);
  if (!root) return { items: [fallback], index: 0 };

  const messageRoot = root.matches(MESSAGE_CONTENT_SELECTOR) ? root : null;
  const markdownRoot = messageRoot ? null : root;
  const triggers = Array.from(
    root.querySelectorAll<HTMLButtonElement>(`button[${PREVIEW_TRIGGER_ATTR}="true"]`),
  ).filter((candidate) =>
    messageRoot
      ? candidate.closest(MESSAGE_CONTENT_SELECTOR) === messageRoot
      : candidate.closest(".md") === markdownRoot,
  );

  const items: GalleryItem[] = [];
  let index = 0;
  for (const candidate of triggers) {
    const image = candidate.querySelector<HTMLImageElement>("img");
    const src = image?.currentSrc || image?.getAttribute("src") || "";
    if (!image || !src) continue;
    if (candidate === trigger) index = items.length;
    items.push({
      src,
      alt: image.alt,
      title: image.title || undefined,
      width: numericAttribute(image, "width"),
      height: numericAttribute(image, "height"),
    });
  }
  return items.length > 0 ? { items, index } : { items: [fallback], index: 0 };
}

export function ImagePreviewGallery({ item, titleFallback, trigger }: Props) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const [gallery, setGallery] = useState<GalleryState | null>(null);
  const [zoomIndex, setZoomIndex] = useState(0);
  const [fittedSize, setFittedSize] = useState<FittedImageSize | null>(null);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);

  const setGalleryIndex = useCallback((index: number) => {
    setZoomIndex(0);
    setFittedSize(null);
    setGallery((current) =>
      current
        ? { ...current, index: Math.max(0, Math.min(index, current.items.length - 1)) }
        : null,
    );
  }, []);

  useEffect(() => {
    if (!zoomed || !gallery) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft" && gallery.index > 0) {
        event.preventDefault();
        setGalleryIndex(gallery.index - 1);
      } else if (event.key === "ArrowRight" && gallery.index < gallery.items.length - 1) {
        event.preventDefault();
        setGalleryIndex(gallery.index + 1);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [gallery, setGalleryIndex, zoomed]);

  const active = gallery?.items[gallery.index] ?? item;
  const hasGallery = (gallery?.items.length ?? 0) > 1;
  const zoomPercent = ZOOM_STEPS[zoomIndex]!;
  const measuredSize = fittedSize?.src === active.src ? fittedSize : null;
  const zoomedSize =
    measuredSize && zoomPercent > 100
      ? {
          width: `${(measuredSize.width * zoomPercent) / 100}px`,
          height: `${(measuredSize.height * zoomPercent) / 100}px`,
          maxWidth: "none",
          maxHeight: "none",
        }
      : undefined;
  const closePreview = () => {
    setZoomed(false);
    setGallery(null);
    setZoomIndex(0);
    setFittedSize(null);
  };
  const saveActiveImage = async () => {
    if (savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    try {
      await saveInlineImage(active.src);
    } catch {
      toast.error(t("message.image.downloadFailed"));
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return (
    <LightboxDialog
      open={zoomed}
      onOpenChange={(open) => {
        setZoomed(open);
        if (!open) {
          setGallery(null);
          setZoomIndex(0);
          setFittedSize(null);
        }
      }}
      title={active.alt || titleFallback}
      kind="media"
      trigger={trigger({
        "data-message-image-preview-trigger": "true",
        onClick: (event) => {
          setZoomIndex(0);
          setFittedSize(null);
          setGallery(collectGallery(event.currentTarget, item));
        },
      })}
    >
      <div {...stylex.props(ig.frame)} data-image-zoom={zoomPercent}>
        <div {...stylex.props(ig.topRight)}>
          <IconButton
            icon="download"
            title={t("message.image.download")}
            aria-busy={saving}
            disabled={saving}
            onClick={() => void saveActiveImage()}
            variant="media"
            size="xl"
          />
          <IconButton
            icon="x"
            title={t("message.image.close")}
            onClick={closePreview}
            variant="media"
            size="xl"
          />
        </div>
        {hasGallery && (
          <>
            <IconButton
              icon="chevron-left"
              title={t("message.image.previous")}
              disabled={gallery!.index === 0}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index - 1);
              }}
              variant="media"
              size="xl"
              className={stylex.props(ig.prev).className}
            />
            <IconButton
              icon="chevron-right"
              title={t("message.image.next")}
              disabled={gallery!.index === gallery!.items.length - 1}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index + 1);
              }}
              variant="media"
              size="xl"
              className={stylex.props(ig.next).className}
            />
          </>
        )}
        <div {...stylex.props(ig.pan)}>
          <div {...stylex.props(ig.centre)}>
            <img
              src={active.src}
              alt={active.alt}
              title={active.title}
              width={active.width}
              height={active.height}
              style={zoomedSize}
              onLoad={(event) => {
                if (zoomPercent !== 100) return;
                const { width, height } = event.currentTarget.getBoundingClientRect();
                if (width > 0 && height > 0) setFittedSize({ src: active.src, width, height });
              }}
              className={cn("media-edge-on-scrim", stylex.props(ig.image).className)}
            />
          </div>
        </div>
        <div {...stylex.props(ig.tray, corner.pill)}>
          <IconButton
            icon="zoom-out"
            title={t("message.image.zoomOut")}
            disabled={zoomIndex === 0}
            onClick={() => setZoomIndex((current) => Math.max(0, current - 1))}
            variant="mediaTray"
            size="xl"
          />
          <span {...stylex.props(ig.zoom, typeStep.uiSm)}>{zoomPercent}%</span>
          <IconButton
            icon="zoom-in"
            title={t("message.image.zoomIn")}
            disabled={zoomIndex === ZOOM_STEPS.length - 1}
            onClick={() => setZoomIndex((current) => Math.min(ZOOM_STEPS.length - 1, current + 1))}
            variant="mediaTray"
            size="xl"
          />
        </div>
      </div>
    </LightboxDialog>
  );
}
