import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  useId,
  useState,
  type KeyboardEventHandler,
  type MouseEventHandler,
  type ReactElement,
} from "react";
import { useT } from "@/lib/i18n/index";
import { useCommandAction } from "@/features/command/public/commandAction";
import { SystemMessage } from "@/ui/index";
import {
  IconButton,
  ScrollRegion,
  LightboxDialog,
  ImageViewport,
  type ImageSource,
} from "@flame/ui";
import { useImageSave } from "@/features/chat/public/imageSave";
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
  header: {
    position: "relative",
    flexShrink: 0,
    minWidth: 0,
    maxHeight: `calc(50dvh - ${space.s6})`,
    paddingBlockStart: space.s3,
    paddingInlineStart: `max(${space.s3}, var(--window-controls-gutter, 0px))`,
    paddingInlineEnd: space.s3,
    zIndex: 1,
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
  },
  toolbar: {
    display: "flex",
    alignItems: "flex-start",
    gap: space.s2,
  },
  // Bounded above the vertically centred navigation so long or enlarged text scrolls instead.
  feedback: {
    maxHeight: "25dvh",
    paddingBlock: 0,
    borderRadius: radius.lg,
    backgroundColor: surface.canvas,
  },
  identity: {
    display: "flex",
    flex: 1,
    alignItems: "flex-start",
    minWidth: 0,
    gap: space.s2,
    paddingBlock: space.s2,
    color: color.onMedia,
  },
  name: {
    flex: 1,
    minWidth: 0,
    paddingBlock: 0,
    maxHeight: "25dvh",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    overscrollBehavior: "contain",
    userSelect: "text",
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
});

const PREVIEW_TRIGGER_ATTR = "data-message-image-preview-trigger";

interface DownloadFailure {
  label: string;
  message: string;
}

// `items` is created once per opening, so its identity names one gallery lifetime.
interface GalleryState {
  items: ImageSource[];
  index: number;
  downloadFailure?: DownloadFailure;
}

interface TriggerProps {
  "data-message-image-preview-trigger": "true";
  onClick: MouseEventHandler<HTMLButtonElement>;
}

interface Props {
  item: ImageSource;
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

function collectGallery(trigger: HTMLButtonElement, fallback: ImageSource): GalleryState {
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

  const items: ImageSource[] = [];
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

export function ImagePreviewGallery({ item, trigger }: Props) {
  const saveInlineImage = useImageSave();
  const t = useT();
  const [gallery, setGallery] = useState<GalleryState | null>(null);
  const failureId = useId();
  const active = gallery?.items[gallery.index] ?? item;
  const activeLabel = active.alt || t("message.image.previewTitle");
  const hasGallery = (gallery?.items.length ?? 0) > 1;
  const failure = gallery?.downloadFailure;
  const save = useCommandAction({
    fallback: t("message.image.downloadFailed"),
    // `run` keeps the sink from the render that started it, so a late failure names that
    // gallery and image and is dropped once that gallery has closed or been replaced.
    onError: (message) => {
      const items = gallery?.items;
      const label = activeLabel;
      setGallery((current) =>
        current && current.items === items
          ? { ...current, downloadFailure: { label, message } }
          : current,
      );
    },
  });
  const download = () => {
    save.run(() => {
      setGallery((current) => current && { ...current, downloadFailure: undefined });
      return saveInlineImage(active.src);
    });
  };
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
        <div {...stylex.props(ig.header)}>
          <div {...stylex.props(ig.toolbar)}>
            <div {...stylex.props(ig.identity, typeStep.uiSm)}>
              <ScrollRegion as="span" label={t("message.image.description")} styles={ig.name}>
                {activeLabel}
              </ScrollRegion>
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
                aria-describedby={failure ? failureId : undefined}
                pending={save.busy}
                onClick={download}
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
          {failure && (
            <ScrollRegion label={t("message.image.downloadFailed")} styles={ig.feedback}>
              <SystemMessage id={failureId} variant="error">
                {`${failure.message}\n${failure.label}`}
              </SystemMessage>
            </ScrollRegion>
          )}
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
        <ImageViewport
          image={active}
          label={activeLabel}
          labels={{ zoomIn: t("message.image.zoomIn"), zoomOut: t("message.image.zoomOut") }}
        />
      </div>
    </LightboxDialog>
  );
}
