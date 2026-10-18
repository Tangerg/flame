import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Pressable } from "@/ui";
import { ImagePreviewGallery } from "../ImagePreviewGallery";
import { UnavailableImage } from "../UnavailableImage";
import { radius, space } from "@flame/ui/tokens.stylex";

const mi = stylex.create({
  trigger: {
    marginBlock: space.s3,
    display: "inline-block",
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    verticalAlign: "top",
    cursor: "zoom-in",
  },
  wide: { maxWidth: "100%" },
  measured: { maxWidth: "min(100%, 44rem)" },
  image: {
    display: "block",
    maxHeight: "calc(var(--spacing) * 50)",
    maxWidth: "100%",
    borderRadius: radius.card,
    objectFit: "contain",
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
    return <UnavailableImage alt={alt} title={title} />;
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
