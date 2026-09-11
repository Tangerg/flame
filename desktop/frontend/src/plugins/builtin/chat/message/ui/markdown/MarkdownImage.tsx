import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Icon, Pressable } from "@/ui";
import { ImagePreviewGallery } from "../ImagePreviewGallery";
import { color, radius, space, surface } from "@/styles/tokens.stylex";

const mi = stylex.create({
  /** The image is the button. `zoom-in` because the press opens it, it does not navigate. */
  trigger: {
    marginBlock: space.s3,
    display: "inline-block",
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    verticalAlign: "top",
    cursor: "zoom-in",
  },
  /** A wide image may take the column. Everything else stops at the reading measure. */
  wide: { maxWidth: "100%" },
  measured: { maxWidth: "min(100%, 44rem)" },
  image: {
    display: "block",
    maxHeight: "calc(var(--spacing) * 50)",
    maxWidth: "100%",
    borderRadius: radius.card,
    objectFit: "contain",
  },
  // An image that will not load still holds a box, so the paragraph around it does not reflow.
  missing: {
    marginBlock: space.s3,
    display: "inline-flex",
    minHeight: space.s24,
    minWidth: space.s24,
    cursor: "default",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: radius.card,
    borderWidth: 0,
    backgroundColor: surface.sunken,
    padding: 0,
    color: color.fgFaint,
  },
});

/**
 * Only a data URI is rendered as an image; anything else takes the `missing` box below.
 *
 * So every `<img>` in this file carries its bytes inline, and NONE of them may be
 * `loading="lazy"`: there is no request to defer, and the attribute costs the one thing that
 * matters here. Measured — a lazy image below the fold stayed `complete: false`, `0x0`, until
 * the reader scrolled to it and it snapped to 240x96, moving the transcript under the line
 * they were reading; its preview `<button>` sat in the tab order at zero size the whole time.
 */
const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;

export function isInlineMarkdownImage(src: string): boolean {
  return INLINE_IMAGE.test(src);
}

interface Props {
  src?: string;
  alt?: string;
  title?: string;
  allowWide?: boolean;
  /**
   * The image is inside a link, so the LINK is the control and this must not be one.
   *
   * `[![badge](img)](url)` is the commonest image in anything an agent quotes, and rendering
   * its own preview trigger there puts a `<button>` inside an `<a target="_blank">`: invalid
   * HTML, two tab stops where the reader sees one badge, and one click that both opens the
   * preview and follows the link. Every other renderer emits `<a><img></a>`, and so does this.
   */
  linked?: boolean;
}

export function MarkdownImage({
  src = "",
  alt = "",
  title,
  allowWide = false,
  linked = false,
}: Props) {
  const t = useT();
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const unavailable = !isInlineMarkdownImage(src) || failedSource === src;

  if (unavailable) {
    // A disabled button is still interactive content, so inside a link it is the same defect.
    // The glyph stays: what it says is "this image did not load", which is still worth saying.
    if (linked) {
      return (
        <span
          role="img"
          aria-label={alt || t("message.image.unavailable")}
          title={title}
          className={stylex.props(mi.missing).className}
        >
          <Icon name="image" size="md" />
        </span>
      );
    }
    return (
      <Pressable
        type="button"
        disabled
        aria-label={alt || t("message.image.unavailable")}
        title={title}
        className={stylex.props(mi.missing).className}
      >
        <Icon name="image" size="md" />
      </Pressable>
    );
  }

  if (linked) {
    return (
      <img
        src={src}
        alt={alt}
        title={title}
        onError={() => setFailedSource(src)}
        className={cn(
          "media-edge",
          stylex.props(mi.image, allowWide ? mi.wide : mi.measured).className,
        )}
      />
    );
  }

  const previewLabel = alt || t("message.image.preview");
  return (
    <ImagePreviewGallery
      item={{ src, alt, title }}
      titleFallback={previewLabel}
      trigger={(previewProps) => (
        <Pressable
          type="button"
          aria-label={previewLabel}
          {...previewProps}
          className={stylex.props(mi.trigger, allowWide ? mi.wide : mi.measured).className}
        >
          <img
            src={src}
            alt={alt}
            title={title}
            onError={() => setFailedSource(src)}
            className={cn("media-edge", stylex.props(mi.image).className)}
          />
        </Pressable>
      )}
    />
  );
}
