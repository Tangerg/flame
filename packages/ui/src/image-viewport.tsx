import * as stylex from "@stylexjs/stylex";
import {
  useCallback,
  useState,
  useRef,
  useEffect,
  useLayoutEffect,
} from "react";
import { ZoomIn, ZoomOut } from "@flame/icons/react";
import { IconButton } from "./icon-button";
import { ScrollRegion } from "./scroll-region";
import {
  color,
  radius,
  space,
  surface,
  type as typeStep,
} from "./tokens.stylex";

export interface ImageSource {
  readonly src: string;
  readonly alt: string;
  readonly title?: string;
  readonly width?: number;
  readonly height?: number;
}
export interface ImageViewportLabels {
  zoomIn: string;
  zoomOut: string;
}
const defaultLabels: ImageViewportLabels = {
  zoomIn: "Zoom in image",
  zoomOut: "Zoom out image",
};
interface FittedImageSize {
  item: ImageSource;
  width: number;
  height: number;
}
const ZOOM_STEPS = [100, 125, 150, 200, 300, 400] as const;

const styles = stylex.create({
  frame: {
    position: "relative",
    display: "flex",
    flexDirection: "column",
    flex: 1,
    minWidth: 0,
    minHeight: 0,
  },
  pan: {
    minHeight: 0,
    flex: 1,
    overflow: "auto",
    paddingInline: space.s4,
    paddingTop: space.s4,
    paddingBottom: space.s16,
    overscrollBehavior: "contain",
    touchAction: "none",
    cursor: {
      default: null,
      ":is([data-pannable])": "grab",
      ":is([data-panning])": "grabbing",
    },
  },
  centre: {
    display: "grid",
    width: "max-content",
    minHeight: "100%",
    minWidth: "100%",
    placeItems: "center",
  },
  image: {
    outline: "1px solid rgb(255 255 255 / 0.1)",
    outlineOffset: "-1px",
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

// Native overflow owns keyboard panning and bounds; Base UI has no image viewport.
export function ImageViewport({
  image: item,
  label,
  labels = defaultLabels,
}: {
  image: ImageSource;
  label: string;
  labels?: ImageViewportLabels;
}) {
  const panRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const [zoomIndex, setZoomIndex] = useState(0);
  const [fittedSize, setFittedSize] = useState<FittedImageSize | null>(null);
  const currentFit = fittedSize?.item === item ? fittedSize : null;
  // WKWebView can derive SVG natural dimensions from rendered size and page zoom.
  // A decoder without a renderer keeps fitting independent of its own output.
  const sourceRef = useRef<HTMLImageElement | null>(null);
  const anchorRef = useRef<{
    x: number;
    y: number;
    clientX: number;
    clientY: number;
  } | null>(null);
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
    if (pan.hasPointerCapture(drag.pointerId))
      pan.releasePointerCapture(drag.pointerId);
  };

  const measure = useCallback(() => {
    const pan = panRef.current;
    const source = sourceRef.current;
    if (!pan || !source) return;
    const { naturalWidth, naturalHeight } = source;
    if (!naturalWidth || !naturalHeight) return;
    const style = getComputedStyle(pan);
    const width =
      pan.clientWidth -
      parseFloat(style.paddingLeft) -
      parseFloat(style.paddingRight);
    const height =
      pan.clientHeight -
      parseFloat(style.paddingTop) -
      parseFloat(style.paddingBottom);
    if (width <= 0 || height <= 0) return;
    const fit = Math.min(1, width / naturalWidth, height / naturalHeight);
    setFittedSize({
      item,
      width: naturalWidth * fit,
      height: naturalHeight * fit,
    });
  }, [item]);

  // Captured items own viewing state even when two entries share the same source.
  useLayoutEffect(() => {
    const pan = panRef.current;
    if (!pan) return;
    anchorRef.current = null;
    setZoomIndex(0);
    setFittedSize(null);
    pan.scrollLeft = 0;
    pan.scrollTop = 0;
    const source = new Image();
    sourceRef.current = source;
    source.addEventListener("load", measure);
    source.src = item.src;
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
      source.removeEventListener("load", measure);
      sourceRef.current = null;
      observer.disconnect();
      window.removeEventListener("blur", cancel);
      stopDrag(true);
    };
  }, [item, measure]);

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
          ? {
              x: (x - box.left) / box.width,
              y: (y - box.top) / box.height,
              clientX: x,
              clientY: y,
            }
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
        Math.max(
          0,
          Math.min(
            ZOOM_STEPS.length - 1,
            zoomIndex + (event.deltaY < 0 ? 1 : -1),
          ),
        ),
        event.clientX,
        event.clientY,
      );
    };
    pan.addEventListener("wheel", wheel, { passive: false });
    return () => pan.removeEventListener("wheel", wheel);
  });

  return (
    <div {...stylex.props(styles.frame)}>
      <ScrollRegion
        ref={panRef}
        styles={styles.pan}
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
          if (zoomPercent <= 100 || event.button !== 0 || dragRef.current)
            return;
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
        <div {...stylex.props(styles.centre)}>
          {/* A new source gets a new element so the previous frame never shows at the new fit. */}
          <img
            key={item.src}
            ref={imageRef}
            src={item.src}
            alt={item.alt}
            title={item.title}
            width={item.width}
            height={item.height}
            draggable={false}
            style={
              currentFit
                ? {
                    width: `${(currentFit.width * zoomPercent) / 100}px`,
                    height: `${(currentFit.height * zoomPercent) / 100}px`,
                    maxWidth: "none",
                    maxHeight: "none",
                  }
                : undefined
            }
            {...stylex.props(styles.image)}
          />
        </div>
      </ScrollRegion>
      <div {...stylex.props(styles.tray)}>
        <IconButton
          icon={ZoomOut}
          aria-label={labels.zoomOut}
          disabled={zoomIndex === 0}
          focusableWhenDisabled
          onClick={() => zoomAt(Math.max(0, zoomIndex - 1))}
          variant="mediaTray"
          size="xl"
        />
        <span {...stylex.props(styles.zoom, typeStep.uiSm)}>
          {zoomPercent}%
        </span>
        <IconButton
          icon={ZoomIn}
          aria-label={labels.zoomIn}
          disabled={zoomIndex === ZOOM_STEPS.length - 1}
          focusableWhenDisabled
          onClick={() => zoomAt(Math.min(ZOOM_STEPS.length - 1, zoomIndex + 1))}
          variant="mediaTray"
          size="xl"
        />
      </div>
    </div>
  );
}
