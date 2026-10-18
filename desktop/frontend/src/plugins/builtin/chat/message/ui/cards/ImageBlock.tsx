import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { useMemo, useState } from "react";
import { Pressable } from "@/ui";
import { imageSizeFromBase64 } from "@/plugins/builtin/chat/message/domain/imageHeader";
import { useT } from "@/lib/i18n";
import { ImagePreviewGallery } from "../ImagePreviewGallery";
import { UnavailableImage } from "../UnavailableImage";
import { radius } from "@flame/ui/tokens.stylex";

const ib = stylex.create({
  frame: {
    display: "block",
    cursor: "zoom-in",
    overflow: "hidden",
    borderRadius: radius.card,
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
  },
  image: {
    width: "auto",
    maxHeight: "calc(var(--spacing) * 64)",
    maxWidth: "100%",
    borderRadius: radius.card,
    objectFit: "contain",
  },
  unavailable: { marginBlock: 0 },
});

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const t = useT();
  const src = `data:${mime};base64,${data}`;
  const size = useMemo(() => imageSizeFromBase64(data), [data]);
  const [failedSource, setFailedSource] = useState<string | null>(null);
  if (failedSource === src) return <UnavailableImage styles={ib.unavailable} />;
  return (
    <ImagePreviewGallery
      item={{ src, alt: "", width: size?.width, height: size?.height }}
      trigger={(previewProps) => (
        <Pressable
          type="button"
          aria-label={t("message.image.view")}
          {...previewProps}
          className={stylex.props(ib.frame).className}
        >
          <img
            src={src}
            alt=""
            width={size?.width}
            height={size?.height}
            onError={() => setFailedSource(src)}
            className={cn("media-edge", stylex.props(ib.image).className)}
          />
        </Pressable>
      )}
    />
  );
}
