import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Icon, Pressable } from "@/ui";
import { ImagePreviewGallery } from "../ImagePreviewGallery";
import { color, radius, space, surface } from "@/styles/tokens.stylex";

const mi = stylex.create({
  image: {
    display: "block",
    maxHeight: "calc(var(--spacing) * 50)",
    maxWidth: "100%",
    borderRadius: radius.card,
    objectFit: "contain",
    boxShadow: "var(--shadow-md)",
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

const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;

export function isInlineMarkdownImage(src: string): boolean {
  return INLINE_IMAGE.test(src);
}

interface Props {
  src?: string;
  alt?: string;
  title?: string;
  allowWide?: boolean;
}

export function MarkdownImage({ src = "", alt = "", title, allowWide = false }: Props) {
  const t = useT();
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const unavailable = !isInlineMarkdownImage(src) || failedSource === src;

  if (unavailable) {
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
          className={cn(
            "my-3 inline-block cursor-zoom-in border-0 bg-transparent p-0 align-top",
            allowWide ? "max-w-full" : "max-w-[min(100%,44rem)]",
          )}
        >
          <img
            src={src}
            alt={alt}
            title={title}
            loading="lazy"
            onError={() => setFailedSource(src)}
            className={cn("media-edge", stylex.props(mi.image).className)}
          />
        </Pressable>
      )}
    />
  );
}
