import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import {
  useState,
  useRef,
  useEffect,
  useLayoutEffect,
  type KeyboardEventHandler,
  type MouseEventHandler,
  type ReactElement,
} from "react";
import { useT } from "@/lib/i18n/index";
import { useCommandAction } from "@/features/command/public/commandAction";
import { LightboxDialog } from "@/ui/index";
import { IconButton, ScrollRegion } from "@flame/ui";
import { saveInlineImage } from "@/features/chat/message/application/imageSave";
import { MESSAGE_CONTENT_SELECTOR } from "@/features/chat/message/ui/messageContent";
import { color, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
const ig = stylex.create({
  frame: {
    position: "relative",
    display: "flex",
    height: "100%",
    width: "100%",
    flexDirection: "column",
  },
  toolbar: {
    position: "absolute",
    top: space.s3,
    left: `max(${space.s3}, var(--window-controls-gutter, 0px))`,
    right: space.s3,
    zIndex: 1,
    display: "flex",
    alignItems: "center",
    gap: space.s2,
  },
  identity: {
    display: "flex",
    alignItems: "center",
    minWidth: 0,
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s2,
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.mediaScrim,
    color: color.onMedia,
  },
  name: {
    minWidth: 0,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  position: { flexShrink: 0, whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" },
  actions: {
    display: "flex",
    alignItems: "center",
    flexShrink: 0,
    marginInlineStart: "auto",
    gap: space.s1,
  },
  prev: { position: "absolute", top: "50%", left: space.s3, zIndex: 1, translate: "0 -50%" },
  next: { position: "absolute", top: "50%", right: space.s3, zIndex: 1, translate: "0 -50%" },
  pan: {
    minHeight: 0,
    flex: 1,
    overflow: "auto",
    paddingInline: space.s4,
    paddingTop: space.s12,
    paddingBottom: space.s16,
    overscrollBehavior: "contain",
    touchAction: "none",
    cursor: { default: null, ":is([data-pannable])": "grab", ":is([data-panning])": "grabbing" },
  },
  centre: {
    display: "grid",
    width: "max-content",
    minHeight: "100%",
    minWidth: "100%",
    placeItems: "center",
  },
  image: {
    display: "block",
    maxHeight: "calc(100dvh - 8rem)",
    maxWidth: "calc(100vw - 2rem)",
    borderRadius: radius.lg,
    objectFit: "contain",
  },
  tray: {
    borderRadius: radius.floatingPanel,
    boxShadow: "var(--shadow-overlay)",
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
  width: number;
  height: number;
}

interface TriggerProps {
  "data-message-image-preview-trigger": "true";
  onClick: MouseEventHandler<HTMLButtonElement>;
}

interface Props {
  item: GalleryItem;
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

// Native overflow owns keyboard panning and bounds; Base UI has no image viewport.
function ImageInspector({ item, label }: { item: GalleryItem; label: string }) {
  const t = useT();
  const panRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const [zoomIndex, setZoomIndex] = useState(0);
  const [fittedSize, setFittedSize] = useState<FittedImageSize | null>(null);
  const anchorRef = useRef<{ x: number; y: number; clientX: number; clientY: number } | null>(null);
  const dragRef = useRef<{
    pointerId: number;
    x: number;
    y: number;
    left: number;
    top: number;
  } | null>(null);
  const zoomPercent = ZOOM_STEPS[zoomIndex]!;

  const stopDrag = (cancel: boolean) => {
    const pan = panRef.current;
    const drag = dragRef.current;
    if (!pan || !drag) return;
    dragRef.current = null;
    pan.removeAttribute("data-panning");
    if (cancel) {
      pan.scrollLeft = drag.left;
      pan.scrollTop = drag.top;
    }
    if (pan.hasPointerCapture(drag.pointerId)) pan.releasePointerCapture(drag.pointerId);
  };

  const measure = () => {
    const pan = panRef.current;
    const image = imageRef.current;
    if (!pan || !image?.naturalWidth || !image.naturalHeight) return;
    const style = getComputedStyle(pan);
    const width = pan.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    const height =
      pan.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
    if (width <= 0 || height <= 0) return;
    const fit = Math.min(1, width / image.naturalWidth, height / image.naturalHeight);
    setFittedSize({ width: image.naturalWidth * fit, height: image.naturalHeight * fit });
  };

  useLayoutEffect(() => {
    const pan = panRef.current;
    if (!pan) return;
    let width = pan.offsetWidth;
    let height = pan.offsetHeight;
    const observer = new ResizeObserver(() => {
      if (width === pan.offsetWidth && height === pan.offsetHeight) return;
      width = pan.offsetWidth;
      height = pan.offsetHeight;
      stopDrag(true);
      anchorRef.current = null;
      setZoomIndex(0);
      pan.scrollLeft = 0;
      pan.scrollTop = 0;
      measure();
    });
    observer.observe(pan, { box: "border-box" });
    const cancel = () => stopDrag(true);
    window.addEventListener("blur", cancel);
    measure();
    return () => {
      observer.disconnect();
      window.removeEventListener("blur", cancel);
      stopDrag(true);
    };
  }, []);

  const zoomAt = (next: number, clientX?: number, clientY?: number) => {
    if (next === zoomIndex) return;
    stopDrag(false);
    const pan = panRef.current;
    const image = imageRef.current;
    if (pan && image) {
      const frame = pan.getBoundingClientRect();
      const box = image.getBoundingClientRect();
      const x = clientX ?? frame.left + pan.clientWidth / 2;
      const y = clientY ?? frame.top + pan.clientHeight / 2;
      anchorRef.current =
        box.width > 0 && box.height > 0
          ? { x: (x - box.left) / box.width, y: (y - box.top) / box.height, clientX: x, clientY: y }
          : null;
    }
    setZoomIndex(next);
  };

  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    anchorRef.current = null;
    const pan = panRef.current;
    const image = imageRef.current;
    if (!anchor || !pan || !image) return;
    const box = image.getBoundingClientRect();
    pan.scrollLeft += box.left + anchor.x * box.width - anchor.clientX;
    pan.scrollTop += box.top + anchor.y * box.height - anchor.clientY;
  }, [zoomIndex]);

  useEffect(() => {
    const pan = panRef.current;
    if (!pan) return;
    const wheel = (event: WheelEvent) => {
      if (event.deltaY === 0) return;
      event.preventDefault();
      zoomAt(
        Math.max(0, Math.min(ZOOM_STEPS.length - 1, zoomIndex + (event.deltaY < 0 ? 1 : -1))),
        event.clientX,
        event.clientY,
      );
    };
    pan.addEventListener("wheel", wheel, { passive: false });
    return () => pan.removeEventListener("wheel", wheel);
  });

  return (
    <>
      <ScrollRegion
        ref={panRef}
        styles={ig.pan}
        data-image-zoom={zoomPercent}
        data-pannable={zoomPercent > 100 ? "" : undefined}
        label={label}
        tabIndex={zoomPercent > 100 ? 0 : -1}
        onKeyDown={(event) => {
          if (zoomPercent <= 100) return;
          const pan = event.currentTarget;
          const step = event.shiftKey ? 0.5 : 0.1;
          switch (event.key) {
            case "ArrowLeft":
              pan.scrollLeft -= pan.clientWidth * step;
              break;
            case "ArrowRight":
              pan.scrollLeft += pan.clientWidth * step;
              break;
            case "ArrowUp":
              pan.scrollTop -= pan.clientHeight * step;
              break;
            case "ArrowDown":
              pan.scrollTop += pan.clientHeight * step;
              break;
            default:
              return;
          }
          event.preventDefault();
          event.stopPropagation();
        }}
        onPointerDown={(event) => {
          if (zoomPercent <= 100 || event.button !== 0 || dragRef.current) return;
          event.preventDefault();
          const pan = event.currentTarget;
          dragRef.current = {
            pointerId: event.pointerId,
            x: event.clientX,
            y: event.clientY,
            left: pan.scrollLeft,
            top: pan.scrollTop,
          };
          pan.setPointerCapture(event.pointerId);
          pan.setAttribute("data-panning", "");
          pan.focus({ preventScroll: true });
        }}
        onPointerMove={(event) => {
          const drag = dragRef.current;
          if (!drag || event.pointerId !== drag.pointerId) return;
          event.currentTarget.scrollLeft = drag.left + drag.x - event.clientX;
          event.currentTarget.scrollTop = drag.top + drag.y - event.clientY;
        }}
        onPointerUp={(event) => {
          if (event.pointerId === dragRef.current?.pointerId) stopDrag(false);
        }}
        onPointerCancel={(event) => {
          if (event.pointerId === dragRef.current?.pointerId) stopDrag(true);
        }}
        onLostPointerCapture={(event) => {
          if (event.pointerId === dragRef.current?.pointerId) stopDrag(true);
        }}
      >
        <div {...stylex.props(ig.centre)}>
          <img
            ref={imageRef}
            src={item.src}
            alt={item.alt}
            title={item.title}
            width={item.width}
            height={item.height}
            draggable={false}
            onLoad={measure}
            style={
              fittedSize
                ? {
                    width: `${(fittedSize.width * zoomPercent) / 100}px`,
                    height: `${(fittedSize.height * zoomPercent) / 100}px`,
                    maxWidth: "none",
                    maxHeight: "none",
                  }
                : undefined
            }
            className={cn("media-edge-on-scrim", stylex.props(ig.image).className)}
          />
        </div>
      </ScrollRegion>
      <div {...stylex.props(ig.tray)}>
        <IconButton
          icon={iconGlyph("zoom-out")}
          aria-label={t("message.image.zoomOut")}
          disabled={zoomIndex === 0}
          focusableWhenDisabled
          onClick={() => zoomAt(Math.max(0, zoomIndex - 1))}
          variant="mediaTray"
          size="xl"
        />
        <span {...stylex.props(ig.zoom, typeStep.uiSm)}>{zoomPercent}%</span>
        <IconButton
          icon={iconGlyph("zoom-in")}
          aria-label={t("message.image.zoomIn")}
          disabled={zoomIndex === ZOOM_STEPS.length - 1}
          focusableWhenDisabled
          onClick={() => zoomAt(Math.min(ZOOM_STEPS.length - 1, zoomIndex + 1))}
          variant="mediaTray"
          size="xl"
        />
      </div>
    </>
  );
}

export function ImagePreviewGallery({ item, trigger }: Props) {
  const t = useT();
  const [gallery, setGallery] = useState<GalleryState | null>(null);
  const save = useCommandAction({ fallback: t("message.image.downloadFailed") });
  const setGalleryIndex = (index: number) => {
    setGallery((current) =>
      current
        ? { ...current, index: Math.max(0, Math.min(index, current.items.length - 1)) }
        : null,
    );
  };
  const onKeyDown: KeyboardEventHandler<HTMLDivElement> = (event) => {
    if (!gallery) return;
    if (event.key === "ArrowLeft" && gallery.index > 0) {
      event.preventDefault();
      setGalleryIndex(gallery.index - 1);
    } else if (event.key === "ArrowRight" && gallery.index < gallery.items.length - 1) {
      event.preventDefault();
      setGalleryIndex(gallery.index + 1);
    }
  };
  const active = gallery?.items[gallery.index] ?? item;
  const activeLabel = active.alt || t("message.image.previewTitle");
  const hasGallery = (gallery?.items.length ?? 0) > 1;
  return (
    <LightboxDialog
      open={gallery !== null}
      onKeyDown={onKeyDown}
      onOpenChange={(open) => {
        if (!open) setGallery(null);
      }}
      title={activeLabel}
      kind="media"
      trigger={trigger({
        "data-message-image-preview-trigger": "true",
        onClick: (event) => setGallery(collectGallery(event.currentTarget, item)),
      })}
    >
      <div {...stylex.props(ig.frame)}>
        <div {...stylex.props(ig.toolbar)}>
          <div {...stylex.props(ig.identity, typeStep.uiSm)}>
            <span aria-hidden="true" title={activeLabel} {...stylex.props(ig.name)}>
              {activeLabel}
            </span>
            {hasGallery && (
              <span role="status" {...stylex.props(ig.position)}>
                {gallery!.index + 1} / {gallery!.items.length}
              </span>
            )}
          </div>
          <div {...stylex.props(ig.actions)}>
            <IconButton
              icon={iconGlyph("download")}
              aria-label={t("message.image.download")}
              aria-busy={save.busy}
              pending={save.busy}
              onClick={() => save.run(() => saveInlineImage(active.src))}
              variant="media"
              size="xl"
            />
            <IconButton
              icon={iconGlyph("x")}
              aria-label={t("message.image.close")}
              onClick={() => setGallery(null)}
              variant="media"
              size="xl"
            />
          </div>
        </div>
        {hasGallery && (
          <>
            <IconButton
              icon={iconGlyph("chevron-left")}
              aria-label={t("message.image.previous")}
              disabled={gallery!.index === 0}
              focusableWhenDisabled
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index - 1);
              }}
              variant="media"
              size="xl"
              className={stylex.props(ig.prev).className}
            />
            <IconButton
              icon={iconGlyph("chevron-right")}
              aria-label={t("message.image.next")}
              disabled={gallery!.index === gallery!.items.length - 1}
              focusableWhenDisabled
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
        <ImageInspector key={gallery?.index ?? "closed"} item={active} label={activeLabel} />
      </div>
    </LightboxDialog>
  );
}
