import { useMemo } from "react";
import { Pressable } from "@/ui";
import { imageSizeFromBase64 } from "@/plugins/builtin/chat/message/domain/imageHeader";
import { useT } from "@/lib/i18n";
import { ImagePreviewGallery } from "../ImagePreviewGallery";

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const t = useT();
  const src = `data:${mime};base64,${data}`;
  const size = useMemo(() => imageSizeFromBase64(data), [data]);
  return (
    <ImagePreviewGallery
      item={{ src, alt: "", width: size?.width, height: size?.height }}
      titleFallback={t("message.image.view")}
      trigger={(previewProps) => (
        <Pressable
          type="button"
          aria-label={t("message.image.view")}
          {...previewProps}
          className="block cursor-zoom-in overflow-hidden rounded-md border-0 bg-transparent p-0"
        >
          <img
            src={src}
            alt=""
            width={size?.width}
            height={size?.height}
            className="max-h-64 max-w-full rounded-md object-contain media-edge"
          />
        </Pressable>
      )}
    />
  );
}
